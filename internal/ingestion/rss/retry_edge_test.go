package rss_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion/rss"
)

func TestAdapterFetchRetriesClientTimeoutWhileBudgetRemains(t *testing.T) {
	client := &sequenceClient{results: []clientResult{
		{err: context.DeadlineExceeded},
		{status: http.StatusOK, body: validFeed},
	}}
	adapter := newRetryAdapter(t, client, noWait)

	if _, err := adapter.Fetch(t.Context()); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if client.calls != 2 {
		t.Errorf("HTTP calls = %d, want 2", client.calls)
	}
}

func TestAdapterFetchRetriesResponseBodyTransportFailure(t *testing.T) {
	client := &sequenceClient{results: []clientResult{
		{status: http.StatusOK, bodyErr: errors.New("unexpected EOF")},
		{status: http.StatusOK, body: validFeed},
	}}
	adapter := newRetryAdapter(t, client, noWait)

	if _, err := adapter.Fetch(t.Context()); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if client.calls != 2 {
		t.Errorf("HTTP calls = %d, want 2", client.calls)
	}
}

func TestAdapterFetchTreatsElapsedRetryAfterDateAsNoDelay(t *testing.T) {
	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	client := &sequenceClient{results: []clientResult{
		{status: http.StatusServiceUnavailable, retryAfter: now.Add(-time.Minute).Format(http.TimeFormat)},
		{status: http.StatusOK, body: validFeed},
	}}
	var delays []time.Duration
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  "example",
		FeedURL: "https://example.com/feed.xml",
		Client:  client,
		Now:     func() time.Time { return now },
		Wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	if _, err := adapter.Fetch(t.Context()); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(delays) != 1 || delays[0] != 0 {
		t.Errorf("retry delays = %v, want [0s]", delays)
	}
}
