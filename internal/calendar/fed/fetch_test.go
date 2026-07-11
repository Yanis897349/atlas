package fed_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdapterFetchEventsClosesResponseBody(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &trackingBody{Reader: bytes.NewReader(fixtureContents(t, "valid.html"))}
			adapter := newAdapter(t, responseClient{response: &http.Response{StatusCode: status, Body: body}}, nil, 0)

			_, _ = adapter.FetchEvents(t.Context())
			if !body.closed {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestAdapterFetchEventsClosesResponseBodyOnTransportFailure(t *testing.T) {
	body := &trackingBody{Reader: http.NoBody}
	adapter := newAdapter(t, errorClient{
		response: &http.Response{StatusCode: http.StatusOK, Body: body},
		err:      errors.New("connection reset"),
	}, nil, 0)

	_, _ = adapter.FetchEvents(t.Context())
	if !body.closed {
		t.Error("response body was not closed")
	}
}

func TestAdapterFetchEventsReportsHTTPFailures(t *testing.T) {
	transportErr := errors.New("connection reset")
	tests := []struct {
		name   string
		client interface {
			Do(*http.Request) (*http.Response, error)
		}
		want string
	}{
		{name: "transport", client: errorClient{err: transportErr}, want: "fetch Federal Reserve calendar"},
		{name: "nil response", client: responseClient{}, want: "nil response"},
		{name: "nil body", client: responseClient{response: &http.Response{StatusCode: http.StatusOK}}, want: "body is nil"},
		{name: "status", client: responseClient{response: &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}}, want: "unexpected HTTP status 503"},
		{name: "read", client: responseClient{response: &http.Response{StatusCode: http.StatusOK, Body: &trackingBody{Reader: failingReader{}}}}, want: "read Federal Reserve calendar"},
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

func TestAdapterFetchEventsRejectsOversizedResponse(t *testing.T) {
	const maxCalendarSize = 10 << 20
	client := responseClient{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       &trackingBody{Reader: bytes.NewReader(make([]byte, maxCalendarSize+1))},
	}}
	adapter := newAdapter(t, client, nil, 0)

	_, err := adapter.FetchEvents(t.Context())
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("FetchEvents() error = %v, want response-size error", err)
	}
}

func TestAdapterFetchEventsPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	adapter := newAdapter(t, contextClient{}, nil, 0)

	_, err := adapter.FetchEvents(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchEvents() error = %v, want context cancellation", err)
	}
}

func TestAdapterFetchEventsHonorsRequestBudget(t *testing.T) {
	adapter := newAdapter(t, contextClient{}, nil, time.Nanosecond)

	_, err := adapter.FetchEvents(t.Context())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FetchEvents() error = %v, want request-budget deadline", err)
	}
}
