package bea_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar/bea"
)

func TestAdapterFetchEventsReportsRetrievalFailures(t *testing.T) {
	const maxResponseSize = 10 << 20
	tests := []struct {
		name   string
		client bea.HTTPClient
		want   string
	}{
		{name: "transport", client: &recordingClient{err: errors.New("connection reset")}, want: "fetch BEA calendar: connection reset"},
		{name: "status", client: &recordingClient{response: &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}}, want: "unexpected HTTP status 503"},
		{name: "read", client: &recordingClient{response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{})}}, want: "read BEA calendar"},
		{
			name: "oversized",
			client: &recordingClient{response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(make([]byte, maxResponseSize+1))),
			}},
			want: "response exceeds",
		},
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
		{
			name:          "request budget",
			requestBudget: time.Nanosecond,
			context:       context.WithCancel,
			want:          context.DeadlineExceeded,
		},
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

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("unexpected EOF")
}

type contextClient struct{}

func (contextClient) Do(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}
