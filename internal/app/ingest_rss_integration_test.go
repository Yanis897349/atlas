package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
)

func TestRunIngestRSSEmptyFeedSkipsEmbeddingProvider(t *testing.T) {
	database := postgrestest.Open(t)
	feed := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/rss+xml")
		_, _ = response.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`))
	}))
	t.Cleanup(feed.Close)
	embeddings := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("embedding provider called for empty feed")
	}))
	t.Cleanup(embeddings.Close)

	stdout := &bytes.Buffer{}
	dependencies := rssIngestionDependencies(database.URL, feed, embeddings, stdout)
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	if err := Run(t.Context(), []string{"ingest-rss"}, dependencies); err != nil {
		t.Fatalf("Run(ingest-rss) error = %v", err)
	}
	if stdout.String() != "database migrations applied\ningested 0 InvestingLive RSS records\n" {
		t.Errorf("stdout = %q, want migration and empty-ingestion success", stdout.String())
	}
}

func TestRunIngestRSSProviderFailurePreservesSourcesWithoutSuccessOutput(t *testing.T) {
	database := postgrestest.Open(t)
	feed := rssTestFeedServer(t)
	embeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "provider unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(embeddings.Close)

	stdout := &bytes.Buffer{}
	dependencies := rssIngestionDependencies(database.URL, feed, embeddings, stdout)
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	stdout.Reset()
	err := Run(t.Context(), []string{"ingest-rss"}, dependencies)
	if err == nil || !strings.Contains(err.Error(), "index ingested InvestingLive RSS source records") ||
		!strings.Contains(err.Error(), "OpenAI Embeddings API returned status 503") {
		t.Fatalf("Run(ingest-rss) error = %v, want contextual provider failure", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no ingestion success output", stdout.String())
	}
	assertRSSPersistenceCounts(t, database, 2, 0)
}

func TestRunIngestRSSEmbeddingPersistenceFailureHasNoSuccessOutput(t *testing.T) {
	database := postgrestest.Open(t)
	feed := rssTestFeedServer(t)
	embeddings := validRSSEmbeddingServer(t)

	stdout := &bytes.Buffer{}
	dependencies := rssIngestionDependencies(database.URL, feed, embeddings, stdout)
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	if _, err := database.Pool.Exec(t.Context(), "DROP TABLE source_record_embeddings"); err != nil {
		t.Fatalf("drop embedding table: %v", err)
	}
	stdout.Reset()
	err := Run(t.Context(), []string{"ingest-rss"}, dependencies)
	if err == nil || !strings.Contains(err.Error(), "persist indexed source record embeddings") {
		t.Fatalf("Run(ingest-rss) error = %v, want contextual embedding persistence failure", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no ingestion success output", stdout.String())
	}
	var sourceCount int
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM source_records").Scan(&sourceCount); err != nil {
		t.Fatalf("count persisted source records: %v", err)
	}
	if sourceCount != 2 {
		t.Errorf("source record count = %d, want 2 after embedding persistence failure", sourceCount)
	}
}

func TestRunIngestRSSPreservesEmbeddingCancellationWithoutSuccessOutput(t *testing.T) {
	database := postgrestest.Open(t)
	feed := rssTestFeedServer(t)
	started := make(chan struct{})
	release := make(chan struct{})
	embeddings := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(embeddings.Close)

	stdout := &bytes.Buffer{}
	dependencies := rssIngestionDependencies(database.URL, feed, embeddings, stdout)
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	stdout.Reset()
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- Run(ctx, []string{"ingest-rss"}, dependencies) }()
	<-started
	cancel()
	err := <-result
	close(release)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "index ingested InvestingLive RSS source records") {
		t.Fatalf("Run(ingest-rss) error = %v, want contextual embedding cancellation", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no ingestion success output", stdout.String())
	}
}

func TestRunIngestRSSSerializesCanonicalTitleAndEmbeddingUpdates(t *testing.T) {
	database := postgrestest.Open(t)
	firstFeed := rssFeedServer(t, "Original title", nil)
	secondFeedStarted := make(chan struct{})
	secondFeed := rssFeedServer(t, "Corrected title", secondFeedStarted)

	firstProviderStarted := make(chan struct{})
	releaseFirstProvider := make(chan struct{})
	firstEmbeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(firstProviderStarted)
		<-releaseFirstProvider
		writeRSSEmbeddingResponse(t, response, []float32{1, 1})
	}))
	t.Cleanup(firstEmbeddings.Close)
	secondEmbeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeRSSEmbeddingResponse(t, response, []float32{9, 9})
	}))
	t.Cleanup(secondEmbeddings.Close)

	firstDependencies := rssIngestionDependencies(database.URL, firstFeed, firstEmbeddings, &bytes.Buffer{})
	firstDependencies.RSSNow = func() time.Time {
		return time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	}
	secondDependencies := rssIngestionDependencies(database.URL, secondFeed, secondEmbeddings, &bytes.Buffer{})
	secondDependencies.RSSNow = func() time.Time {
		return time.Date(2026, time.July, 12, 12, 1, 0, 0, time.UTC)
	}
	if err := Run(t.Context(), []string{"migrate"}, firstDependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	firstResult := make(chan error, 1)
	go func() { firstResult <- Run(t.Context(), []string{"ingest-rss"}, firstDependencies) }()
	waitForSignal(t, firstProviderStarted, "first embedding provider request")

	secondResult := make(chan error, 1)
	go func() { secondResult <- Run(t.Context(), []string{"ingest-rss"}, secondDependencies) }()
	select {
	case <-secondFeedStarted:
		t.Fatal("second RSS cycle fetched before the first cycle released its ingestion lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirstProvider)
	if err := waitForResult(t, firstResult, "first RSS cycle"); err != nil {
		t.Fatalf("first Run(ingest-rss) error = %v", err)
	}
	if err := waitForResult(t, secondResult, "second RSS cycle"); err != nil {
		t.Fatalf("second Run(ingest-rss) error = %v", err)
	}

	var title, vector string
	if err := database.Pool.QueryRow(t.Context(), `
SELECT sr.title, sre.embedding::text
FROM source_records sr
JOIN source_record_embeddings sre ON sre.source_record_id = sr.id
`).Scan(&title, &vector); err != nil {
		t.Fatalf("load final canonical source and embedding: %v", err)
	}
	if title != "Corrected title" || vector != "[9,9]" {
		t.Errorf("final source and embedding = (%q, %q), want corrected title and its vector", title, vector)
	}
}

func rssTestFeedServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/rss+xml")
		_, _ = response.Write([]byte(testFeed))
	}))
	t.Cleanup(server.Close)
	return server
}

func rssFeedServer(t *testing.T, title string, started chan<- struct{}) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if started != nil {
			close(started)
		}
		response.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(response, `<?xml version="1.0"?><rss version="2.0"><channel><item><guid>shared-story</guid><link>https://investinglive.com/shared-story</link><title>%s</title><pubDate>Fri, 10 Jul 2026 12:00:00 GMT</pubDate></item></channel></rss>`, title)
	}))
	t.Cleanup(server.Close)
	return server
}

func validRSSEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var input struct {
			Texts []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode embedding request: %v", err)
		}
		data := make([]map[string]any, len(input.Texts))
		for index := range input.Texts {
			data[index] = map[string]any{"object": "embedding", "index": index, "embedding": []float32{float32(index + 1), 1}}
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(map[string]any{
			"object": "list",
			"model":  "rss-embedding-model",
			"data":   data,
		}); err != nil {
			t.Errorf("encode embedding response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeRSSEmbeddingResponse(t *testing.T, response http.ResponseWriter, vector []float32) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(map[string]any{
		"object": "list",
		"model":  "rss-embedding-model",
		"data": []map[string]any{{
			"object": "embedding", "index": 0, "embedding": vector,
		}},
	}); err != nil {
		t.Errorf("encode embedding response: %v", err)
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForResult(t *testing.T, result <-chan error, description string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		return nil
	}
}

func rssIngestionDependencies(
	databaseURL string,
	feed *httptest.Server,
	embeddings *httptest.Server,
	stdout *bytes.Buffer,
) Dependencies {
	return Dependencies{
		Getenv: func(name string) string {
			return map[string]string{
				"ATLAS_DATABASE_URL":           databaseURL,
				"ATLAS_OPENAI_API_KEY":         "rss-secret",
				"ATLAS_OPENAI_EMBEDDING_MODEL": "rss-embedding-model",
			}[name]
		},
		RSSHTTPClient:            feed.Client(),
		RSSFeedURL:               feed.URL,
		OpenAIHTTPClient:         embeddings.Client(),
		OpenAIEmbeddingsEndpoint: embeddings.URL + "/v1/embeddings",
		Stdout:                   stdout,
	}
}

func assertRSSPersistenceCounts(
	t *testing.T,
	database postgrestest.Database,
	wantSources int,
	wantEmbeddings int,
) {
	t.Helper()
	var sourceCount, embeddingCount int
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM source_records").Scan(&sourceCount); err != nil {
		t.Fatalf("count source records: %v", err)
	}
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM source_record_embeddings").Scan(&embeddingCount); err != nil {
		t.Fatalf("count source record embeddings: %v", err)
	}
	if sourceCount != wantSources || embeddingCount != wantEmbeddings {
		t.Errorf("persistence counts = (%d sources, %d embeddings), want (%d, %d)", sourceCount, embeddingCount, wantSources, wantEmbeddings)
	}
}
