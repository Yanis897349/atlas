package rsscmd

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

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/search"
	searchopenai "github.com/Yanis897349/atlas/internal/search/openai"
)

func TestRunEmptyFeedSkipsEmbeddingProvider(t *testing.T) {
	database := migratedDatabase(t)
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
	if err := Run(
		t.Context(),
		database.Pool,
		testEmbedder(t, embeddings),
		stdout,
		testDependencies(feed),
	); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() != "ingested 0 InvestingLive RSS records\n" {
		t.Errorf("stdout = %q, want empty-ingestion success", stdout.String())
	}
}

func TestRunProviderFailurePreservesSourcesWithoutSuccessOutput(t *testing.T) {
	database := migratedDatabase(t)
	feed := testFeedServer(t)
	embeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "provider unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(embeddings.Close)

	stdout := &bytes.Buffer{}
	err := Run(t.Context(), database.Pool, testEmbedder(t, embeddings), stdout, testDependencies(feed))
	if err == nil || !strings.Contains(err.Error(), "index ingested InvestingLive RSS source records") ||
		!strings.Contains(err.Error(), "OpenAI Embeddings API returned status 503") {
		t.Fatalf("Run() error = %v, want contextual provider failure", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no ingestion success output", stdout.String())
	}
	assertPersistenceCounts(t, database, 2, 0)
}

func TestRunEmbeddingPersistenceFailureHasNoSuccessOutput(t *testing.T) {
	database := migratedDatabase(t)
	feed := testFeedServer(t)
	embeddings := validEmbeddingServer(t)
	if _, err := database.Pool.Exec(t.Context(), "DROP TABLE source_record_embeddings"); err != nil {
		t.Fatalf("drop embedding table: %v", err)
	}

	stdout := &bytes.Buffer{}
	err := Run(t.Context(), database.Pool, testEmbedder(t, embeddings), stdout, testDependencies(feed))
	if err == nil || !strings.Contains(err.Error(), "persist indexed source record embeddings") {
		t.Fatalf("Run() error = %v, want contextual embedding persistence failure", err)
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

func TestRunPreservesEmbeddingCancellationWithoutSuccessOutput(t *testing.T) {
	database := migratedDatabase(t)
	feed := testFeedServer(t)
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
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	embedder := testEmbedder(t, embeddings)
	go func() {
		result <- Run(ctx, database.Pool, embedder, stdout, testDependencies(feed))
	}()
	waitForSignal(t, started, "embedding provider request")
	cancel()
	err := waitForResult(t, result, "canceled RSS cycle")
	close(release)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "index ingested InvestingLive RSS source records") {
		t.Fatalf("Run() error = %v, want contextual embedding cancellation", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no ingestion success output", stdout.String())
	}
}

func TestRunReleasesIngestionLockAfterFailure(t *testing.T) {
	database := migratedDatabase(t)
	feed := testFeedServer(t)
	failingEmbeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "provider unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(failingEmbeddings.Close)

	err := Run(
		t.Context(),
		database.Pool,
		testEmbedder(t, failingEmbeddings),
		&bytes.Buffer{},
		testDependencies(feed),
	)
	if err == nil {
		t.Fatal("first Run() error = nil, want provider failure")
	}

	successfulEmbeddings := validEmbeddingServer(t)
	stdout := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := Run(
		ctx,
		database.Pool,
		testEmbedder(t, successfulEmbeddings),
		stdout,
		testDependencies(feed),
	); err != nil {
		t.Fatalf("second Run() error = %v, want successful cycle after lock release", err)
	}
	if stdout.String() != "ingested 2 InvestingLive RSS records\n" {
		t.Errorf("stdout = %q, want successful retry output", stdout.String())
	}
	assertPersistenceCounts(t, database, 2, 2)
}

func TestRunSerializesCanonicalTitleAndEmbeddingUpdates(t *testing.T) {
	database := migratedDatabase(t)
	firstFeed := feedServer(t, "Original title", nil)
	secondFeedStarted := make(chan struct{})
	secondFeed := feedServer(t, "Corrected title", secondFeedStarted)

	firstProviderStarted := make(chan struct{})
	releaseFirstProvider := make(chan struct{})
	firstEmbeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		close(firstProviderStarted)
		<-releaseFirstProvider
		writeEmbeddingResponse(t, response, []float32{1, 1})
	}))
	t.Cleanup(firstEmbeddings.Close)
	secondEmbeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeEmbeddingResponse(t, response, []float32{9, 9})
	}))
	t.Cleanup(secondEmbeddings.Close)

	firstDependencies := testDependencies(firstFeed)
	firstDependencies.Now = func() time.Time {
		return time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	}
	secondDependencies := testDependencies(secondFeed)
	secondDependencies.Now = func() time.Time {
		return time.Date(2026, time.July, 12, 12, 1, 0, 0, time.UTC)
	}
	firstEmbedder := testEmbedder(t, firstEmbeddings)
	secondEmbedder := testEmbedder(t, secondEmbeddings)

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- Run(
			t.Context(),
			database.Pool,
			firstEmbedder,
			&bytes.Buffer{},
			firstDependencies,
		)
	}()
	waitForSignal(t, firstProviderStarted, "first embedding provider request")

	secondResult := make(chan error, 1)
	go func() {
		secondResult <- Run(
			t.Context(),
			database.Pool,
			secondEmbedder,
			&bytes.Buffer{},
			secondDependencies,
		)
	}()
	select {
	case <-secondFeedStarted:
		t.Fatal("second RSS cycle fetched before the first cycle released its ingestion lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirstProvider)
	if err := waitForResult(t, firstResult, "first RSS cycle"); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := waitForResult(t, secondResult, "second RSS cycle"); err != nil {
		t.Fatalf("second Run() error = %v", err)
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

func migratedDatabase(t *testing.T) postgrestest.Database {
	t.Helper()
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return database
}

func testEmbedder(t *testing.T, server *httptest.Server) search.Embedder {
	t.Helper()
	embedder, err := searchopenai.NewEmbedder(searchopenai.Config{
		APIKey:   "rss-secret",
		Model:    "rss-embedding-model",
		Client:   server.Client(),
		Endpoint: server.URL + "/v1/embeddings",
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}
	return embedder
}

func testDependencies(feed *httptest.Server) Dependencies {
	return Dependencies{
		HTTPClient: feed.Client(),
		FeedURL:    feed.URL,
	}
}

func testFeedServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/rss+xml")
		_, _ = response.Write([]byte(testFeed))
	}))
	t.Cleanup(server.Close)
	return server
}

func feedServer(t *testing.T, title string, started chan<- struct{}) *httptest.Server {
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

func validEmbeddingServer(t *testing.T) *httptest.Server {
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
			data[index] = map[string]any{
				"object": "embedding", "index": index, "embedding": []float32{float32(index + 1), 1},
			}
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

func writeEmbeddingResponse(t *testing.T, response http.ResponseWriter, vector []float32) {
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

func assertPersistenceCounts(
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
		t.Errorf(
			"persistence counts = (%d sources, %d embeddings), want (%d, %d)",
			sourceCount,
			embeddingCount,
			wantSources,
			wantEmbeddings,
		)
	}
}

const testFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item><guid>story-1</guid><link>https://investinglive.com/story-1</link><title>Story one</title><pubDate>Fri, 10 Jul 2026 12:00:00 GMT</pubDate></item>
  <item><guid>story-2</guid><link>https://investinglive.com/story-2</link><title>Story two</title><pubDate>Fri, 10 Jul 2026 13:00:00 GMT</pubDate></item>
</channel></rss>`
