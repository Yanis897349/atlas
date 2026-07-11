package census_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar/census"
)

func TestAdapterFetchEventsReportsRetrievalFailures(t *testing.T) {
	tests := []struct {
		name   string
		client census.HTTPClient
		want   string
	}{
		{name: "transport", client: &recordingClient{err: errors.New("connection reset")}, want: "fetch Census calendar: connection reset"},
		{name: "status", client: &recordingClient{response: &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}}, want: "unexpected HTTP status 503"},
		{name: "oversized", client: &recordingClient{contents: make([]byte, (10<<20)+1)}, want: "response exceeds"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newAdapter(t, test.client, nil, 0)
			_, err := adapter.FetchEvents(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FetchEvents() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestAdapterFetchEventsPropagatesCancellationAndDeadline(t *testing.T) {
	tests := []struct {
		name          string
		requestBudget time.Duration
		context       func(context.Context) (context.Context, context.CancelFunc)
		want          error
	}{
		{
			name: "cancellation",
			context: func(parent context.Context) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(parent)
				cancel()
				return ctx, func() {}
			},
			want: context.Canceled,
		},
		{name: "request budget", requestBudget: time.Nanosecond, context: context.WithCancel, want: context.DeadlineExceeded},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.context(t.Context())
			defer cancel()
			adapter := newAdapter(t, contextClient{}, nil, test.requestBudget)
			_, err := adapter.FetchEvents(ctx)
			if !errors.Is(err, test.want) {
				t.Fatalf("FetchEvents() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewAdapterValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config census.Config
	}{
		{name: "relative URL", config: census.Config{CalendarURL: "/calendar.html"}},
		{name: "unsupported URL scheme", config: census.Config{CalendarURL: "file:///calendar.html"}},
		{name: "negative request budget", config: census.Config{RequestBudget: -time.Second}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := census.NewAdapter(test.config); err == nil {
				t.Fatal("NewAdapter() error = nil, want validation error")
			}
		})
	}
	if adapter, err := census.NewAdapter(census.Config{}); err != nil || adapter == nil {
		t.Fatalf("NewAdapter(defaults) = %v, %v, want configured adapter", adapter, err)
	}
}

type contextClient struct{}

func (contextClient) Do(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}
