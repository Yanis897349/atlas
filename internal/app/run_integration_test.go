package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
)

func TestRunIngestsRSSIdempotentlyEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	feed := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/rss+xml")
		_, _ = response.Write([]byte(testFeed))
	}))
	t.Cleanup(feed.Close)

	var providerCalls atomic.Int32
	embeddings := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		providerCalls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("embedding request = %s %s, want POST /v1/embeddings", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer rss-secret" {
			t.Errorf("embedding authorization = %q, want RSS credential", request.Header.Get("Authorization"))
		}
		var input struct {
			Model string   `json:"model"`
			Texts []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode embedding request: %v", err)
		}
		if input.Model != "rss-embedding-model" || !reflect.DeepEqual(input.Texts, []string{"Story one", "Story two"}) {
			t.Errorf("embedding input = %#v, want exact feed-order titles", input)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"object":"list","model":"rss-embedding-model","data":[{"object":"embedding","index":1,"embedding":[3,4]},{"object":"embedding","index":0,"embedding":[1,2]}]}`))
	}))
	t.Cleanup(embeddings.Close)

	dependencies := Dependencies{
		Getenv: func(name string) string {
			return map[string]string{
				"ATLAS_DATABASE_URL":           database.URL,
				"ATLAS_OPENAI_API_KEY":         "rss-secret",
				"ATLAS_OPENAI_EMBEDDING_MODEL": "rss-embedding-model",
			}[name]
		},
		RSSHTTPClient:            feed.Client(),
		RSSFeedURL:               feed.URL,
		OpenAIHTTPClient:         embeddings.Client(),
		OpenAIEmbeddingsEndpoint: embeddings.URL + "/v1/embeddings",
	}
	for range 2 {
		if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
			t.Fatalf("Run(migrate) error = %v", err)
		}
	}
	for iteration := range 2 {
		stdout := &bytes.Buffer{}
		dependencies.Stdout = stdout
		if err := Run(t.Context(), []string{"ingest-rss"}, dependencies); err != nil {
			t.Fatalf("Run(ingest-rss) error = %v", err)
		}
		if stdout.String() != "ingested 2 InvestingLive RSS records\n" {
			t.Errorf("ingestion %d output = %q, want success count", iteration+1, stdout.String())
		}
	}

	var count int
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM source_records").Scan(&count); err != nil {
		t.Fatalf("count source records: %v", err)
	}
	if count != 2 {
		t.Errorf("source record count = %d, want 2", count)
	}
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM source_record_embeddings").Scan(&count); err != nil {
		t.Fatalf("count source record embeddings: %v", err)
	}
	if count != 2 {
		t.Errorf("source record embedding count = %d, want 2", count)
	}
	var titles []string
	rows, err := database.Pool.Query(t.Context(), `
SELECT sr.title
FROM source_record_embeddings sre
JOIN source_records sr ON sr.id = sre.source_record_id
ORDER BY sr.published_at
`)
	if err != nil {
		t.Fatalf("load indexed source records: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatalf("scan indexed title: %v", err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate indexed titles: %v", err)
	}
	if !reflect.DeepEqual(titles, []string{"Story one", "Story two"}) {
		t.Errorf("indexed titles = %#v, want canonical feed records", titles)
	}
	var provider, model, createdBy, updatedBy string
	var vector string
	if err := database.Pool.QueryRow(t.Context(), `
SELECT provider, model, embedding::text, created_by, updated_by
FROM source_record_embeddings
LIMIT 1
`).Scan(&provider, &model, &vector, &createdBy, &updatedBy); err != nil {
		t.Fatalf("load embedding provenance: %v", err)
	}
	if provider != "openai" || model != "rss-embedding-model" || strings.Count(vector, ",") != 1 ||
		createdBy != rssIngestionActor || updatedBy != rssIngestionActor {
		t.Errorf("embedding metadata = (%q, %q, %q, %q, %q), want normalized provenance, dimension 2, and RSS audit actor", provider, model, vector, createdBy, updatedBy)
	}
	if providerCalls.Load() != 2 {
		t.Errorf("provider calls = %d, want one per ingestion cycle", providerCalls.Load())
	}
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM economic_events").Scan(&count); err != nil {
		t.Fatalf("count economic events: %v", err)
	}
	if count != 0 {
		t.Errorf("economic event count = %d, want 0", count)
	}
}

func TestRunReportsIngestionFailureAndCancellation(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: func(name string) string {
		return map[string]string{
			"ATLAS_DATABASE_URL":           database.URL,
			"ATLAS_OPENAI_API_KEY":         "rss-secret",
			"ATLAS_OPENAI_EMBEDDING_MODEL": "rss-embedding-model",
		}[name]
	}}
	dependencies.RSSWait = func(context.Context, time.Duration) error { return nil }
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	t.Run("HTTP failure", func(t *testing.T) {
		feed := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
		}))
		t.Cleanup(feed.Close)

		dependencies.RSSHTTPClient = feed.Client()
		dependencies.RSSFeedURL = feed.URL
		err := Run(t.Context(), []string{"ingest-rss"}, dependencies)
		if err == nil || !strings.Contains(err.Error(), "ingest InvestingLive RSS: fetch source records") {
			t.Fatalf("Run(ingest-rss) error = %v, want contextual fetch error", err)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		started := make(chan struct{})
		feed := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			close(started)
			<-request.Context().Done()
		}))
		t.Cleanup(feed.Close)

		ctx, cancel := context.WithCancel(t.Context())
		dependencies.RSSHTTPClient = feed.Client()
		dependencies.RSSFeedURL = feed.URL
		result := make(chan error, 1)
		go func() {
			result <- Run(ctx, []string{"ingest-rss"}, dependencies)
		}()
		<-started
		cancel()

		err := <-result
		if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "ingest InvestingLive RSS") {
			t.Fatalf("Run(ingest-rss) error = %v, want contextual cancellation", err)
		}
	})
}

func TestRunListsUpcomingEventsEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	repository, err := calendarpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(4 * time.Hour)
	events := []calendar.Event{
		commandEvent("before", calendar.RegionUnitedStates, windowStart.Add(-time.Microsecond)),
		commandEvent("start", calendar.RegionUnitedStates, windowStart),
		commandEvent("middle-b", calendar.RegionUnitedStates, windowStart.Add(2*time.Hour)),
		commandEvent("middle-a", calendar.RegionUnitedStates, windowStart.Add(2*time.Hour)),
		commandEvent("end", calendar.RegionUnitedStates, windowEnd),
		commandEvent("after", calendar.RegionUnitedStates, windowEnd.Add(time.Microsecond)),
		commandEvent("other-region", calendar.RegionEurozone, windowStart.Add(time.Hour)),
	}
	storedByExternalID := make(map[string]calendar.StoredEvent, len(events))
	for _, event := range events {
		stored, persistErr := repository.UpsertEvent(t.Context(), event, "calendar-ingestion")
		if persistErr != nil {
			t.Fatalf("UpsertEvent(%q) error = %v", event.ExternalEventID, persistErr)
		}
		storedByExternalID[event.ExternalEventID] = stored
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	err = Run(t.Context(), []string{
		"upcoming-events",
		"--region", "united_states",
		"--from", "2026-08-01T10:00:00+02:00",
		"--to", "2026-08-01T12:00:00Z",
		"--limit", "3",
	}, dependencies)
	if err != nil {
		t.Fatalf("Run(upcoming-events) error = %v", err)
	}

	middleIDs := []string{storedByExternalID["middle-a"].ID, storedByExternalID["middle-b"].ID}
	sort.Strings(middleIDs)
	wantIDs := []string{storedByExternalID["start"].ID, middleIDs[0], middleIDs[1]}
	wantExternalIDs := []string{"start"}
	for _, id := range middleIDs {
		if id == storedByExternalID["middle-a"].ID {
			wantExternalIDs = append(wantExternalIDs, "middle-a")
		} else {
			wantExternalIDs = append(wantExternalIDs, "middle-b")
		}
	}

	var output []upcomingEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command JSON: %v", err)
	}
	if len(output) != 3 {
		t.Fatalf("output count = %d, want 3", len(output))
	}
	for index := range output {
		if output[index].ID != wantIDs[index] || output[index].ExternalEventID != wantExternalIDs[index] {
			t.Errorf("output[%d] identity = (%q, %q), want (%q, %q)", index, output[index].ID, output[index].ExternalEventID, wantIDs[index], wantExternalIDs[index])
		}
		if output[index].Source != "example-calendar" || output[index].SourceURL == "" {
			t.Errorf("output[%d] source citation = (%q, %q), want populated", index, output[index].Source, output[index].SourceURL)
		}
		if !strings.HasSuffix(output[index].ScheduledAt, "Z") || !strings.HasSuffix(output[index].RetrievedAt, "Z") {
			t.Errorf("output[%d] times = (%q, %q), want UTC", index, output[index].ScheduledAt, output[index].RetrievedAt)
		}
	}

	stdout.Reset()
	err = Run(t.Context(), []string{
		"upcoming-events",
		"--region", "united_states",
		"--from", "2026-08-01T08:00:00Z",
		"--to", "2026-08-01T12:00:00Z",
		"--limit", "10",
	}, dependencies)
	if err != nil {
		t.Fatalf("Run(upcoming-events inclusive window) error = %v", err)
	}
	output = nil
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode inclusive command JSON: %v", err)
	}
	if len(output) != 4 || output[0].ExternalEventID != "start" || output[len(output)-1].ExternalEventID != "end" {
		t.Errorf("inclusive output boundary events = %#v, want start through end", output)
	}
}

func commandEvent(externalEventID string, region calendar.Region, scheduledAt time.Time) calendar.Event {
	return calendar.Event{
		Source:          "example-calendar",
		ExternalEventID: externalEventID,
		Name:            "Economic event " + externalEventID,
		Region:          region,
		Type:            calendar.EventTypeGDP,
		ScheduledAt:     scheduledAt,
		SourceURL:       "https://example.com/calendar/" + externalEventID,
		RetrievedAt:     time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC),
	}
}

func applicationDatabaseEnv(databaseURL string) func(string) string {
	return func(name string) string {
		if name == "ATLAS_DATABASE_URL" {
			return databaseURL
		}
		return ""
	}
}

const testFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item><guid>story-1</guid><link>https://investinglive.com/story-1</link><title>Story one</title><pubDate>Fri, 10 Jul 2026 12:00:00 GMT</pubDate></item>
  <item><guid>story-2</guid><link>https://investinglive.com/story-2</link><title>Story two</title><pubDate>Fri, 10 Jul 2026 13:00:00 GMT</pubDate></item>
</channel></rss>`
