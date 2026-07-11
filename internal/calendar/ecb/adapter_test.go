package ecb_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/ecb"
)

func TestAdapterFetchEventsNormalizesMonetaryPolicyDecisions(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 11, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	client := fixtureClient(t, "valid.html")
	adapter := newAdapter(t, client, func() time.Time { return retrievedAt }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}

	want := []calendar.Event{
		{
			Source:          ecb.Source,
			ExternalEventID: "ecb-2026-02-05",
			Name:            "European Central Bank Interest Rate Decision",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeInterestRateDecision,
			ScheduledAt:     time.Date(2026, time.February, 5, 13, 15, 0, 0, time.UTC),
			SourceURL:       ecb.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          ecb.Source,
			ExternalEventID: "ecb-2026-07-23",
			Name:            "European Central Bank Interest Rate Decision",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeInterestRateDecision,
			ScheduledAt:     time.Date(2026, time.July, 23, 12, 15, 0, 0, time.UTC),
			SourceURL:       ecb.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
	}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("FetchEvents() = %#v, want %#v", events, want)
	}
	if client.requests != 1 {
		t.Errorf("HTTP requests = %d, want 1", client.requests)
	}
	if client.method != http.MethodGet || client.accept != "text/html" {
		t.Errorf("request = %s with Accept %q, want GET with text/html", client.method, client.accept)
	}
	if client.userAgent != "Atlas (+https://github.com/Yanis897349/atlas)" {
		t.Errorf("User-Agent = %q, want identifiable Atlas project URL", client.userAgent)
	}
}

func TestAdapterFetchEventsKeepsIdentityAcrossRetrievals(t *testing.T) {
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	adapter := newAdapter(t, fixtureClient(t, "valid.html"), func() time.Time { return now }, 0)

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
			t.Error("RetrievedAt did not change between fetches")
		}
	}
}

func TestAdapterFetchEventsCollapsesRepeatedIdentities(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "repeated.html"), nil, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].ExternalEventID != "ecb-2026-07-23" {
		t.Fatalf("FetchEvents() = %#v, want one July 23 decision", events)
	}
}

func TestAdapterFetchEventsIgnoresUnsupportedRowsBeforeDateValidation(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "unsupported.html"), nil, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("FetchEvents() = %#v, want no events", events)
	}
}

func TestAdapterFetchEventsRejectsMalformedCalendarData(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{name: "invalid fixture date", html: string(fixtureContents(t, "malformed.html")), want: "invalid meeting date"},
		{name: "missing schedule", html: "<html><body>not a calendar</body></html>", want: "meeting schedule is required"},
		{name: "missing list", html: `<div class="definition-list"></div>`, want: "schedule list is required"},
		{name: "empty list", html: `<div class="definition-list"><dl></dl></div>`, want: "schedule entries are required"},
		{name: "description without date", html: `<div class="definition-list"><dl><dd>meeting</dd></dl></div>`, want: "description is missing a date"},
		{name: "date without description", html: `<div class="definition-list"><dl><dt>23/07/2026</dt></dl></div>`, want: "date is missing a description"},
		{name: "repeated date", html: `<div class="definition-list"><dl><dt>22/07/2026</dt><dt>23/07/2026</dt><dd>meeting</dd></dl></div>`, want: "date is missing a description"},
		{name: "wrong date format", html: supportedRow("2026-07-23"), want: "invalid meeting date"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newAdapter(t, &recordingClient{contents: []byte(test.html)}, nil, 0)
			_, err := adapter.FetchEvents(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FetchEvents() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestNewAdapterValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config ecb.Config
	}{
		{name: "relative URL", config: ecb.Config{CalendarURL: "/calendar.html"}},
		{name: "unsupported URL scheme", config: ecb.Config{CalendarURL: "file:///calendar.html"}},
		{name: "negative request budget", config: ecb.Config{RequestBudget: -time.Second}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ecb.NewAdapter(test.config); err == nil {
				t.Fatal("NewAdapter() error = nil, want validation error")
			}
		})
	}
	if adapter, err := ecb.NewAdapter(ecb.Config{}); err != nil || adapter == nil {
		t.Fatalf("NewAdapter(defaults) = %v, %v, want configured adapter", adapter, err)
	}
}

func supportedRow(date string) string {
	return `<div class="definition-list"><dl><dt>` + date + `</dt><dd>` +
		`Governing Council of the ECB: monetary policy meeting in Frankfurt (Day 2), followed by press conference` +
		`</dd></dl></div>`
}

func newAdapter(t *testing.T, client ecb.HTTPClient, now func() time.Time, requestBudget time.Duration) *ecb.Adapter {
	t.Helper()
	adapter, err := ecb.NewAdapter(ecb.Config{
		CalendarURL:   "https://example.com/ecb.html",
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
	contents  []byte
	response  *http.Response
	err       error
	requests  int
	method    string
	accept    string
	userAgent string
}

func (client *recordingClient) Do(request *http.Request) (*http.Response, error) {
	client.requests++
	client.method = request.Method
	client.accept = request.Header.Get("Accept")
	client.userAgent = request.Header.Get("User-Agent")
	if client.response != nil || client.err != nil {
		return client.response, client.err
	}
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
