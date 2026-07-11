package eurostat_test

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/eurostat"
)

func TestAdapterFetchEventsNormalizesSupportedReleases(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 11, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	client := fixtureClient(t, "valid.json")
	adapter := newAdapter(t, client, func() time.Time { return retrievedAt }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}

	want := []calendar.Event{
		{
			Source:          eurostat.Source,
			ExternalEventID: "eurostat-gdp-2025-q4-preliminary-flash",
			Name:            "Preliminary flash estimate GDP - EU and euro area",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.January, 30, 11, 0, 0, 0, time.UTC),
			SourceURL:       eurostat.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          eurostat.Source,
			ExternalEventID: "eurostat-retail-sales-2025-12",
			Name:            "Retail trade",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeRetailSales,
			ScheduledAt:     time.Date(2026, time.February, 5, 11, 0, 0, 0, time.UTC),
			SourceURL:       eurostat.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          eurostat.Source,
			ExternalEventID: "eurostat-gdp-2025-q4-flash",
			Name:            "Flash estimate GDP and employment - EU and euro area",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.February, 13, 11, 0, 0, 0, time.UTC),
			SourceURL:       eurostat.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          eurostat.Source,
			ExternalEventID: "eurostat-gdp-2025-q4-main-aggregates",
			Name:            "GDP main aggregates and employment",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.March, 6, 11, 0, 0, 0, time.UTC),
			SourceURL:       eurostat.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          eurostat.Source,
			ExternalEventID: "eurostat-gdp-2025-q4-main-aggregates-update",
			Name:            "GDP main aggregates and employment - update",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.April, 20, 11, 0, 0, 123_000_000, time.UTC),
			SourceURL:       eurostat.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          eurostat.Source,
			ExternalEventID: "eurostat-retail-sales-2026-03",
			Name:            "Retail trade",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypeRetailSales,
			ScheduledAt:     time.Date(2026, time.May, 7, 11, 0, 0, 0, time.UTC),
			SourceURL:       eurostat.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
	}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("FetchEvents() = %#v, want %#v", events, want)
	}
	if client.requests != 1 || client.method != http.MethodGet {
		t.Errorf("requests = %d using %s, want one GET", client.requests, client.method)
	}
	if client.accept != "application/json" || client.userAgent != "Atlas (+https://github.com/Yanis897349/atlas)" {
		t.Errorf("request headers = Accept %q, User-Agent %q", client.accept, client.userAgent)
	}
	assertCalendarQuery(t, client.query)
}

func TestAdapterFetchEventsKeepsIdentityAcrossRetrievals(t *testing.T) {
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	client := fixtureClient(t, "valid.json")
	adapter := newAdapter(t, client, func() time.Time { return now }, 0)

	first, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("first FetchEvents() error = %v", err)
	}
	now = now.Add(time.Hour)
	client.contents = bytes.Replace(client.contents, []byte("2026-01-30T11:00:00Z"), []byte("2026-01-31T11:00:00Z"), 1)
	client.contents = bytes.Replace(client.contents, []byte("2026-02-05T11:00Z"), []byte("2026-02-06T11:00Z"), 1)
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
	if first[0].ScheduledAt.Equal(second[0].ScheduledAt) {
		t.Error("GDP ScheduledAt did not reflect the corrected release date")
	}
	if first[1].ScheduledAt.Equal(second[1].ScheduledAt) {
		t.Error("retail sales ScheduledAt did not reflect the corrected release date")
	}
}

func TestAdapterFetchEventsCapturesRetrievalTimeAfterResponse(t *testing.T) {
	queryTime := time.Date(2026, time.December, 31, 23, 59, 0, 0, time.FixedZone("CET", 60*60))
	retrievedAt := queryTime.Add(2 * time.Minute)
	now := queryTime
	client := fixtureClient(t, "valid.json")
	client.onDo = func() { now = retrievedAt }
	adapter := newAdapter(t, client, func() time.Time { return now }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	for _, event := range events {
		if !event.RetrievedAt.Equal(retrievedAt.UTC()) {
			t.Errorf("RetrievedAt = %v, want post-response time %v", event.RetrievedAt, retrievedAt.UTC())
		}
	}
	if got := client.query.Get("start"); got != "2026-01-01T00:00:00+01:00" {
		t.Errorf("query start = %q, want window based on pre-request year 2026", got)
	}
}

func TestAdapterFetchEventsCollapsesRepeatedIdentities(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "repeated.json"), nil, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 2 || events[0].ExternalEventID != "eurostat-gdp-2026-q2-preliminary-flash" || events[1].ExternalEventID != "eurostat-retail-sales-2026-05" {
		t.Fatalf("FetchEvents() = %#v, want one GDP and one retail sales event", events)
	}
	want := []time.Time{
		time.Date(2026, time.July, 30, 11, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 6, 11, 0, 0, 0, time.UTC),
	}
	for index := range events {
		if !events[index].ScheduledAt.Equal(want[index]) {
			t.Errorf("event %d ScheduledAt = %v, want first-seen time %v", index, events[index].ScheduledAt, want[index])
		}
	}
}

func TestAdapterFetchEventsIgnoresUnsupportedReleasesBeforeFieldValidation(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "unsupported.json"), nil, 0)

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
		json string
		want string
	}{
		{name: "invalid JSON", json: `[`, want: "unexpected end of JSON input"},
		{name: "null list", json: `null`, want: "release list is required"},
		{name: "missing title", json: `[{}]`, want: "release 1 title is required"},
		{name: "invalid fixture period", json: string(fixtureContents(t, "malformed.json")), want: "invalid GDP reference period"},
		{name: "missing period", json: supportedRelease("", "2026-04-30T11:00:00Z"), want: "invalid GDP reference period"},
		{name: "invalid quarter", json: supportedRelease("Q5/2026", "2026-04-30T11:00:00Z"), want: "invalid GDP reference period"},
		{name: "missing retail period", json: retailSalesRelease("", "2026-04-08T11:00:00Z"), want: "invalid retail sales reference period"},
		{name: "invalid retail period", json: retailSalesRelease("February 2026 revised", "2026-04-08T11:00:00Z"), want: "invalid retail sales reference period"},
		{name: "missing start", json: supportedRelease("Q1/2026", ""), want: "release start is required"},
		{name: "invalid start", json: supportedRelease("Q1/2026", "April 30, 2026"), want: "invalid release start"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newAdapter(t, &recordingClient{contents: []byte(test.json)}, nil, 0)
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
		config eurostat.Config
	}{
		{name: "relative URL", config: eurostat.Config{EventsURL: "/events.json"}},
		{name: "unsupported URL scheme", config: eurostat.Config{EventsURL: "file:///events.json"}},
		{name: "negative request budget", config: eurostat.Config{RequestBudget: -time.Second}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := eurostat.NewAdapter(test.config); err == nil {
				t.Fatal("NewAdapter() error = nil, want validation error")
			}
		})
	}
	if adapter, err := eurostat.NewAdapter(eurostat.Config{}); err != nil || adapter == nil {
		t.Fatalf("NewAdapter(defaults) = %v, %v, want configured adapter", adapter, err)
	}
}

func assertCalendarQuery(t *testing.T, query url.Values) {
	t.Helper()
	want := map[string]string{
		"authorExclude":   "",
		"authorInclude":   "",
		"category":        "0",
		"end":             "2027-01-01T00:00:00+01:00",
		"isEuroindicator": "true",
		"keywords":        "",
		"start":           "2026-01-01T00:00:00+01:00",
		"theme":           "0",
	}
	for key, value := range want {
		if query.Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, query.Get(key), value)
		}
	}
}

func supportedRelease(period, start string) string {
	return `[{"period":"` + period + `","start":"` + start + `","title":"Preliminary flash estimate GDP - EU and euro area"}]`
}

func retailSalesRelease(period, start string) string {
	return `[{"period":"` + period + `","start":"` + start + `","title":"Retail trade"}]`
}

func newAdapter(t *testing.T, client eurostat.HTTPClient, now func() time.Time, requestBudget time.Duration) *eurostat.Adapter {
	t.Helper()
	adapter, err := eurostat.NewAdapter(eurostat.Config{
		EventsURL:     "https://example.com/events.json",
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
	onDo      func()
	requests  int
	method    string
	query     url.Values
	accept    string
	userAgent string
}

func (client *recordingClient) Do(request *http.Request) (*http.Response, error) {
	client.requests++
	client.method = request.Method
	client.query = request.URL.Query()
	client.accept = request.Header.Get("Accept")
	client.userAgent = request.Header.Get("User-Agent")
	if client.onDo != nil {
		client.onDo()
	}
	if client.response != nil || client.err != nil {
		return client.response, client.err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(client.contents))}, nil
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
