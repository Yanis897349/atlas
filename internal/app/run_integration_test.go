package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
)

func TestRunIngestsRSSIdempotentlyEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	feed := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/rss+xml")
		_, _ = response.Write([]byte(testFeed))
	}))
	t.Cleanup(feed.Close)

	dependencies := Dependencies{
		Getenv:        applicationDatabaseEnv(database.URL),
		RSSHTTPClient: feed.Client(),
		RSSFeedURL:    feed.URL,
	}
	for range 2 {
		if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
			t.Fatalf("Run(migrate) error = %v", err)
		}
	}
	for range 2 {
		if err := Run(t.Context(), []string{"ingest-rss"}, dependencies); err != nil {
			t.Fatalf("Run(ingest-rss) error = %v", err)
		}
	}

	var count int
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM source_records").Scan(&count); err != nil {
		t.Fatalf("count source records: %v", err)
	}
	if count != 2 {
		t.Errorf("source record count = %d, want 2", count)
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
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
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
