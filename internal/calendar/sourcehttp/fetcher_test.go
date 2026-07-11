package sourcehttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFetcherSendsBoundedCalendarRequest(t *testing.T) {
	body := &trackingBody{Reader: strings.NewReader("calendar")}
	client := &recordingClient{response: &http.Response{StatusCode: http.StatusOK, Body: body}}
	fetcher := newFetcher(t, Config{
		Resource: "Example calendar",
		URL:      " https://example.com/calendar ",
		Accept:   "text/calendar",
		Client:   client,
	})

	got, err := fetcher.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(got) != "calendar" {
		t.Errorf("Fetch() = %q, want calendar", got)
	}
	if client.method != http.MethodGet || client.url != "https://example.com/calendar" {
		t.Errorf("request = %s %s, want GET https://example.com/calendar", client.method, client.url)
	}
	if client.accept != "text/calendar" || client.userAgent != defaultUserAgent {
		t.Errorf("request headers = (Accept %q, User-Agent %q), want (%q, %q)", client.accept, client.userAgent, "text/calendar", defaultUserAgent)
	}
	if !body.closed {
		t.Error("response body was not closed")
	}
}

func TestFetcherSendsConfiguredUserAgent(t *testing.T) {
	client := &recordingClient{response: &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}}
	fetcher := newFetcher(t, Config{
		Resource:  "Example calendar",
		URL:       "https://example.com/calendar",
		Accept:    "text/html",
		UserAgent: " Compatible calendar client ",
		Client:    client,
	})

	if _, err := fetcher.Fetch(t.Context()); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if client.userAgent != "Compatible calendar client" {
		t.Errorf("User-Agent = %q, want configured value", client.userAgent)
	}
}

func TestFetcherSendsEncodedQueryWithoutDiscardingConfiguredValues(t *testing.T) {
	client := &recordingClient{response: &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}}
	fetcher := newFetcher(t, Config{
		Resource: "Example calendar",
		URL:      "https://example.com/calendar?language=en&keywords=old",
		Accept:   "application/json",
		Client:   client,
	})

	_, err := fetcher.FetchWithQuery(t.Context(), url.Values{
		"keywords": {"GDP and employment"},
		"start":    {"2026-01-01T00:00:00+01:00"},
	})
	if err != nil {
		t.Fatalf("FetchWithQuery() error = %v", err)
	}
	want := "https://example.com/calendar?keywords=GDP+and+employment&language=en&start=2026-01-01T00%3A00%3A00%2B01%3A00"
	if client.url != want {
		t.Errorf("request URL = %q, want %q", client.url, want)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing resource", config: Config{URL: "https://example.com", Accept: "text/html"}, want: "resource is required"},
		{name: "relative URL", config: Config{Resource: "calendar", URL: "/calendar", Accept: "text/html"}, want: "invalid calendar URL"},
		{name: "unsupported scheme", config: Config{Resource: "calendar", URL: "file:///calendar", Accept: "text/html"}, want: "invalid calendar URL"},
		{name: "missing Accept", config: Config{Resource: "calendar", URL: "https://example.com"}, want: "Accept media type is required"},
		{name: "negative budget", config: Config{Resource: "calendar", URL: "https://example.com", Accept: "text/html", RequestBudget: -time.Second}, want: "request budget must not be negative"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestFetcherClosesResponseBodyOnTransportFailure(t *testing.T) {
	body := &trackingBody{Reader: http.NoBody}
	fetcher := newFetcher(t, Config{
		Resource: "Example calendar",
		URL:      "https://example.com/calendar",
		Accept:   "text/calendar",
		Client: &recordingClient{
			response: &http.Response{StatusCode: http.StatusOK, Body: body},
			err:      errors.New("connection reset"),
		},
	})

	_, _ = fetcher.Fetch(t.Context())
	if !body.closed {
		t.Error("response body was not closed")
	}
}

func TestFetcherReportsRetrievalFailures(t *testing.T) {
	tests := []struct {
		name   string
		client Client
		want   string
	}{
		{name: "transport", client: &recordingClient{err: errors.New("connection reset")}, want: "fetch Example calendar: connection reset"},
		{name: "nil response", client: &recordingClient{}, want: "nil response"},
		{name: "nil body", client: &recordingClient{response: &http.Response{StatusCode: http.StatusOK}}, want: "body is nil"},
		{name: "status", client: &recordingClient{response: &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}}, want: "unexpected HTTP status 503"},
		{name: "upstream challenge", client: &recordingClient{response: &http.Response{StatusCode: http.StatusAccepted, Header: http.Header{"X-Amzn-Waf-Action": {"challenge"}}, Body: http.NoBody}}, want: "upstream request challenge"},
		{name: "read", client: &recordingClient{response: &http.Response{StatusCode: http.StatusOK, Body: &trackingBody{Reader: failingReader{}}}}, want: "read Example calendar"},
		{
			name: "oversized",
			client: &recordingClient{response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       &trackingBody{Reader: bytes.NewReader(make([]byte, maxResponseSize+1))},
			}},
			want: "response exceeds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fetcher := newFetcher(t, Config{
				Resource: "Example calendar",
				URL:      "https://example.com/calendar",
				Accept:   "text/calendar",
				Client:   test.client,
			})
			_, err := fetcher.Fetch(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Fetch() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestFetcherPropagatesCancellationAndDeadline(t *testing.T) {
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
			fetcher := newFetcher(t, Config{
				Resource:      "Example calendar",
				URL:           "https://example.com/calendar",
				Accept:        "text/calendar",
				Client:        contextClient{},
				RequestBudget: test.requestBudget,
			})

			_, err := fetcher.Fetch(ctx)
			if !errors.Is(err, test.want) {
				t.Fatalf("Fetch() error = %v, want %v", err, test.want)
			}
		})
	}
}

func newFetcher(t *testing.T, config Config) *Fetcher {
	t.Helper()
	fetcher, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return fetcher
}

type recordingClient struct {
	response  *http.Response
	err       error
	method    string
	url       string
	accept    string
	userAgent string
}

func (client *recordingClient) Do(request *http.Request) (*http.Response, error) {
	client.method = request.Method
	client.url = request.URL.String()
	client.accept = request.Header.Get("Accept")
	client.userAgent = request.Header.Get("User-Agent")
	return client.response, client.err
}

type contextClient struct{}

func (contextClient) Do(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("unexpected EOF")
}

type trackingBody struct {
	io.Reader
	closed bool
}

func (body *trackingBody) Close() error {
	body.closed = true
	return nil
}
