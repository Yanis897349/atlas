package bls_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/bls"
)

func TestAdapterFetchEventsNormalizesSupportedReleases(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 10, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	client := fixtureClient(t, "valid.ics")
	adapter := newAdapter(t, client, func() time.Time { return retrievedAt }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("FetchEvents() returned %d events, want 2", len(events))
	}

	want := []calendar.Event{
		{
			Source:          bls.Source,
			ExternalEventID: "cpi-2026-08",
			Name:            "Consumer Price Index",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeInflation,
			ScheduledAt:     time.Date(2026, time.August, 12, 12, 30, 0, 0, time.UTC),
			SourceURL:       bls.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          bls.Source,
			ExternalEventID: "employment-2026-07",
			Name:            "Employment Situation",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeEmployment,
			ScheduledAt:     time.Date(2026, time.August, 7, 12, 30, 0, 0, time.UTC),
			SourceURL:       bls.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
	}
	for index := range want {
		if events[index] != want[index] {
			t.Errorf("FetchEvents()[%d] = %#v, want %#v", index, events[index], want[index])
		}
	}
	if client.requests != 1 {
		t.Errorf("HTTP requests = %d, want 1", client.requests)
	}
	if client.method != http.MethodGet || client.accept != "text/calendar" {
		t.Errorf("request = %s with Accept %q, want GET with text/calendar", client.method, client.accept)
	}
}

func TestAdapterFetchEventsKeepsIdentityAcrossRetrievals(t *testing.T) {
	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	adapter := newAdapter(t, fixtureClient(t, "valid.ics"), func() time.Time { return now }, 0)

	first, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("first FetchEvents() error = %v", err)
	}
	now = now.Add(time.Hour)
	second, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("second FetchEvents() error = %v", err)
	}
	for index := range first {
		if first[index].ExternalEventID != second[index].ExternalEventID {
			t.Errorf("ExternalEventID changed from %q to %q", first[index].ExternalEventID, second[index].ExternalEventID)
		}
		if first[index].RetrievedAt.Equal(second[index].RetrievedAt) {
			t.Errorf("RetrievedAt did not change between fetches")
		}
	}
}

func TestAdapterFetchEventsCollapsesRepeatedUIDs(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "repeated.ics"), nil, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("FetchEvents() returned %d events, want 1", len(events))
	}
	wantScheduledAt := time.Date(2026, time.August, 7, 12, 30, 0, 0, time.UTC)
	if !events[0].ScheduledAt.Equal(wantScheduledAt) {
		t.Errorf("ScheduledAt = %v, want first occurrence %v", events[0].ScheduledAt, wantScheduledAt)
	}
}

func TestAdapterFetchEventsIgnoresUnsupportedReleasesBeforeValidation(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "unsupported.ics"), nil, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("FetchEvents() = %#v, want no events", events)
	}
}

func TestAdapterFetchEventsRejectsMalformedInput(t *testing.T) {
	for _, fixture := range []string{"malformed.ics", "invalid-supported.ics", "invalid-start.ics"} {
		t.Run(fixture, func(t *testing.T) {
			adapter := newAdapter(t, fixtureClient(t, fixture), nil, 0)
			if events, err := adapter.FetchEvents(t.Context()); err == nil {
				t.Fatalf("FetchEvents() = %#v, nil; want malformed input error", events)
			}
		})
	}
}

func TestAdapterFetchEventsClosesResponseBody(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &trackingBody{Reader: bytes.NewReader(fixtureContents(t, "valid.ics"))}
			adapter := newAdapter(t, responseClient{status: status, body: body}, nil, 0)

			_, _ = adapter.FetchEvents(t.Context())
			if !body.closed {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestAdapterFetchEventsReportsHTTPFailures(t *testing.T) {
	transportErr := errors.New("connection reset")
	tests := []struct {
		name   string
		client bls.HTTPClient
		want   string
	}{
		{name: "transport", client: errorClient{err: transportErr}, want: "fetch BLS calendar"},
		{name: "status", client: responseClient{status: http.StatusServiceUnavailable}, want: "unexpected HTTP status 503"},
		{name: "read", client: responseClient{body: failingReader{}}, want: "read BLS calendar"},
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
	adapter := newAdapter(t, responseClient{body: bytes.NewReader(make([]byte, maxCalendarSize+1))}, nil, 0)

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

func TestNewAdapterValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config bls.Config
	}{
		{name: "relative URL", config: bls.Config{CalendarURL: "/calendar.ics"}},
		{name: "unsupported URL scheme", config: bls.Config{CalendarURL: "file:///calendar.ics"}},
		{name: "negative request budget", config: bls.Config{RequestBudget: -time.Second}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := bls.NewAdapter(test.config); err == nil {
				t.Fatal("NewAdapter() error = nil, want validation error")
			}
		})
	}
}

func newAdapter(
	t *testing.T,
	client bls.HTTPClient,
	now func() time.Time,
	requestBudget time.Duration,
) *bls.Adapter {
	t.Helper()
	adapter, err := bls.NewAdapter(bls.Config{
		CalendarURL:   "https://example.com/bls.ics",
		Client:        client,
		Now:           now,
		RequestBudget: requestBudget,
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	return adapter
}

type recordingClient struct {
	contents []byte
	requests int
	method   string
	accept   string
}

func (client *recordingClient) Do(request *http.Request) (*http.Response, error) {
	client.requests++
	client.method = request.Method
	client.accept = request.Header.Get("Accept")
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(client.contents)),
	}, nil
}

func fixtureClient(t *testing.T, name string) *recordingClient {
	t.Helper()
	return &recordingClient{contents: fixtureContents(t, name)}
}

func fixtureContents(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return contents
}

type errorClient struct {
	err error
}

func (client errorClient) Do(*http.Request) (*http.Response, error) {
	return nil, client.err
}

type responseClient struct {
	status int
	body   io.Reader
}

func (client responseClient) Do(*http.Request) (*http.Response, error) {
	status := client.status
	if status == 0 {
		status = http.StatusOK
	}
	body := client.body
	if body == nil {
		body = http.NoBody
	}
	responseBody, ok := body.(io.ReadCloser)
	if !ok {
		responseBody = io.NopCloser(body)
	}
	return &http.Response{StatusCode: status, Body: responseBody}, nil
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
