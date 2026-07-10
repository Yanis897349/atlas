package rss_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion/rss"
)

func TestAdapterFetchRetriesTransientFailures(t *testing.T) {
	client := &sequenceClient{results: []clientResult{
		{err: errors.New("connection reset")},
		{status: http.StatusTooManyRequests, retryAfter: "7"},
		{status: http.StatusOK, body: validFeed},
	}}
	var delays []time.Duration
	adapter := newRetryAdapter(t, client, func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})

	records, err := adapter.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(records) != 1 || client.calls != 3 {
		t.Fatalf("Fetch() returned %d records after %d calls, want 1 record after 3 calls", len(records), client.calls)
	}
	wantDelays := []time.Duration{time.Second, 7 * time.Second}
	if len(delays) != len(wantDelays) || delays[0] != wantDelays[0] || delays[1] != wantDelays[1] {
		t.Errorf("retry delays = %v, want %v", delays, wantDelays)
	}
}

func TestAdapterFetchRetriesTransientServerStatuses(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable, 599} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := &sequenceClient{results: []clientResult{
				{status: status},
				{status: http.StatusOK, body: validFeed},
			}}
			adapter := newRetryAdapter(t, client, noWait)

			if _, err := adapter.Fetch(t.Context()); err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if client.calls != 2 {
				t.Errorf("HTTP calls = %d, want 2", client.calls)
			}
		})
	}
}

func TestAdapterFetchStopsAfterBoundedAttempts(t *testing.T) {
	client := &sequenceClient{results: []clientResult{{status: http.StatusServiceUnavailable}}}
	adapter := newRetryAdapter(t, client, noWait)

	_, err := adapter.Fetch(t.Context())
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("Fetch() error = %v, want bounded-attempt error", err)
	}
	if client.calls != 3 {
		t.Errorf("HTTP calls = %d, want 3", client.calls)
	}
}

func TestAdapterFetchDoesNotRetryPermanentOrParsingFailures(t *testing.T) {
	tests := []struct {
		name   string
		result clientResult
	}{
		{name: "permanent HTTP status", result: clientResult{status: http.StatusNotFound}},
		{name: "malformed RSS", result: clientResult{status: http.StatusOK, body: []byte("<rss>not closed")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &sequenceClient{results: []clientResult{test.result}}
			waits := 0
			adapter := newRetryAdapter(t, client, func(context.Context, time.Duration) error {
				waits++
				return nil
			})

			if _, err := adapter.Fetch(t.Context()); err == nil {
				t.Fatal("Fetch() error = nil, want terminal error")
			}
			if client.calls != 1 || waits != 0 {
				t.Errorf("HTTP calls = %d and waits = %d, want one call and no waits", client.calls, waits)
			}
		})
	}
}

func TestAdapterFetchDoesNotRetryCancellation(t *testing.T) {
	client := &sequenceClient{results: []clientResult{{err: context.Canceled}}}
	adapter := newRetryAdapter(t, client, func(context.Context, time.Duration) error {
		t.Fatal("Wait() called for canceled fetch")
		return nil
	})

	_, err := adapter.Fetch(t.Context())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch() error = %v, want context cancellation", err)
	}
	if client.calls != 1 {
		t.Errorf("HTTP calls = %d, want 1", client.calls)
	}
}

func TestAdapterFetchHonorsRequestBudgetBeforeRetryAfter(t *testing.T) {
	client := &sequenceClient{results: []clientResult{{
		status:     http.StatusTooManyRequests,
		retryAfter: "60",
	}}}
	waits := 0
	adapter, err := rss.NewAdapter(rss.Config{
		Source:        "example",
		FeedURL:       "https://example.com/feed.xml",
		Client:        client,
		RequestBudget: 5 * time.Second,
		Wait: func(context.Context, time.Duration) error {
			waits++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	_, err = adapter.Fetch(t.Context())
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "request budget") {
		t.Fatalf("Fetch() error = %v, want request-budget deadline error", err)
	}
	if client.calls != 1 || waits != 0 {
		t.Errorf("HTTP calls = %d and waits = %d, want one call and no wait beyond budget", client.calls, waits)
	}
}

func TestAdapterFetchPropagatesCancellationWhileWaiting(t *testing.T) {
	client := &sequenceClient{results: []clientResult{{err: errors.New("temporary network failure")}}}
	adapter := newRetryAdapter(t, client, func(context.Context, time.Duration) error {
		return context.Canceled
	})

	_, err := adapter.Fetch(t.Context())
	if !errors.Is(err, context.Canceled) || client.calls != 1 {
		t.Fatalf("Fetch() = (%v, %d calls), want cancellation after one call", err, client.calls)
	}
}

func newRetryAdapter(t *testing.T, client rss.HTTPClient, wait func(context.Context, time.Duration) error) *rss.Adapter {
	t.Helper()
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  "example",
		FeedURL: "https://example.com/feed.xml",
		Client:  client,
		Wait:    wait,
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	return adapter
}

func noWait(context.Context, time.Duration) error {
	return nil
}

type clientResult struct {
	status     int
	retryAfter string
	body       []byte
	bodyErr    error
	err        error
}

type sequenceClient struct {
	results []clientResult
	calls   int
}

func (client *sequenceClient) Do(*http.Request) (*http.Response, error) {
	index := client.calls
	client.calls++
	if index >= len(client.results) {
		index = len(client.results) - 1
	}
	result := client.results[index]
	if result.err != nil {
		return nil, result.err
	}
	status := result.status
	if status == 0 {
		status = http.StatusOK
	}
	header := make(http.Header)
	if result.retryAfter != "" {
		header.Set("Retry-After", result.retryAfter)
	}
	body := io.Reader(bytes.NewReader(result.body))
	if result.bodyErr != nil {
		body = failingReader{err: result.bodyErr}
	}
	return &http.Response{
		Status:     http.StatusText(status),
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(body),
	}, nil
}

type failingReader struct {
	err error
}

func (reader failingReader) Read([]byte) (int, error) {
	return 0, reader.err
}

var validFeed = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><item>
<guid>story-1</guid><link>https://example.com/story-1</link>
<title>Story one</title><pubDate>Fri, 10 Jul 2026 12:00:00 GMT</pubDate>
</item></channel></rss>`)
