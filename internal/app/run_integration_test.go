package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunIngestsRSSIdempotentlyEndToEnd(t *testing.T) {
	databaseURL, applicationPool := isolatedApplicationDatabase(t)
	feed := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/rss+xml")
		_, _ = response.Write([]byte(testFeed))
	}))
	t.Cleanup(feed.Close)

	dependencies := Dependencies{
		Getenv:     applicationDatabaseEnv(databaseURL),
		HTTPClient: feed.Client(),
		FeedURL:    feed.URL,
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
	if err := applicationPool.QueryRow(t.Context(), "SELECT count(*) FROM source_records").Scan(&count); err != nil {
		t.Fatalf("count source records: %v", err)
	}
	if count != 2 {
		t.Errorf("source record count = %d, want 2", count)
	}
	if err := applicationPool.QueryRow(t.Context(), "SELECT count(*) FROM economic_events").Scan(&count); err != nil {
		t.Fatalf("count economic events: %v", err)
	}
	if count != 0 {
		t.Errorf("economic event count = %d, want 0", count)
	}
}

func TestRunReportsIngestionFailureAndCancellation(t *testing.T) {
	databaseURL, _ := isolatedApplicationDatabase(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(databaseURL)}
	dependencies.RSSWait = func(context.Context, time.Duration) error { return nil }
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	t.Run("HTTP failure", func(t *testing.T) {
		feed := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
		}))
		t.Cleanup(feed.Close)

		dependencies.HTTPClient = feed.Client()
		dependencies.FeedURL = feed.URL
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
		dependencies.HTTPClient = feed.Client()
		dependencies.FeedURL = feed.URL
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

func isolatedApplicationDatabase(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()

	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set")
	}
	adminPool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	t.Cleanup(adminPool.Close)

	schema := "atlas_app_test_" + randomSchemaSuffix(t)
	if _, err := adminPool.Exec(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	applicationURL := parsed.String()
	applicationPool, err := pgxpool.New(t.Context(), applicationURL)
	if err != nil {
		t.Fatalf("connect to isolated application schema: %v", err)
	}
	t.Cleanup(applicationPool.Close)
	return applicationURL, applicationPool
}

func applicationDatabaseEnv(databaseURL string) func(string) string {
	return func(name string) string {
		if name == "ATLAS_DATABASE_URL" {
			return databaseURL
		}
		return ""
	}
}

func randomSchemaSuffix(t *testing.T) string {
	t.Helper()
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate test schema name: %v", err)
	}
	return hex.EncodeToString(value)
}

const testFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item><guid>story-1</guid><link>https://investinglive.com/story-1</link><title>Story one</title><pubDate>Fri, 10 Jul 2026 12:00:00 GMT</pubDate></item>
  <item><guid>story-2</guid><link>https://investinglive.com/story-2</link><title>Story two</title><pubDate>Fri, 10 Jul 2026 13:00:00 GMT</pubDate></item>
</channel></rss>`
