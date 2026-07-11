package bea_test

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
	"github.com/Yanis897349/atlas/internal/calendar/bea"
)

func TestAdapterFetchEventsNormalizesNationalGDPReleases(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 11, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	client := fixtureClient(t, "valid.html")
	adapter := newAdapter(t, client, func() time.Time { return retrievedAt }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}

	want := []calendar.Event{
		{
			Source:          bea.Source,
			ExternalEventID: "bea-gdp-2025-q4-advance",
			Name:            "GDP (Advance Estimate), 4th Quarter and Year 2025",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.February, 20, 13, 30, 0, 0, time.UTC),
			SourceURL:       bea.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          bea.Source,
			ExternalEventID: "bea-gdp-2025-q4-second",
			Name:            "GDP (Second Estimate), 4th Quarter and Year 2025",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.March, 13, 12, 30, 0, 0, time.UTC),
			SourceURL:       bea.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          bea.Source,
			ExternalEventID: "bea-gdp-2026-q1-third",
			Name:            "GDP (Third Estimate), Industries, Corporate Profits, State GDP, and State Personal Income, 1st Quarter 2026",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.June, 25, 12, 30, 0, 0, time.UTC),
			SourceURL:       bea.CalendarURL,
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
	client := fixtureClient(t, "valid.html")
	adapter := newAdapter(t, client, func() time.Time { return now }, 0)

	first, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("first FetchEvents() error = %v", err)
	}
	now = now.Add(time.Hour)
	client.contents = bytes.Replace(client.contents, []byte("February 20"), []byte("February 21"), 1)
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
		t.Error("ScheduledAt did not reflect the corrected release date")
	}
}

func TestAdapterFetchEventsCollapsesRepeatedIdentities(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "repeated.html"), nil, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].ExternalEventID != "bea-gdp-2026-q2-advance" {
		t.Fatalf("FetchEvents() = %#v, want one 2026 Q2 advance estimate", events)
	}
	if want := time.Date(2026, time.July, 30, 12, 30, 0, 0, time.UTC); !events[0].ScheduledAt.Equal(want) {
		t.Errorf("ScheduledAt = %v, want first-seen time %v", events[0].ScheduledAt, want)
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
		{name: "invalid fixture date", html: string(fixtureContents(t, "malformed.html")), want: "invalid release date and time"},
		{name: "missing table", html: "<html><body>not a calendar</body></html>", want: "schedule table is required"},
		{name: "missing heading", html: `<table id="release-schedule-table"><tbody><tr></tr></tbody></table>`, want: "schedule heading is required"},
		{name: "missing year heading", html: `<table id="release-schedule-table"><thead><tr><td>Year</td></tr></thead><tbody><tr></tr></tbody></table>`, want: "year heading is required"},
		{name: "invalid year heading", html: scheduleHTML("Year 20XX", supportedRow("July 30", "8:30 AM", "GDP (Advance Estimate), 2nd Quarter 2026")), want: "invalid release schedule year heading"},
		{name: "missing body", html: `<table id="release-schedule-table"><thead><tr><th>Year 2026</th></tr></thead></table>`, want: "schedule body is required"},
		{name: "empty body", html: scheduleHTML("Year 2026", ""), want: "schedule rows are required"},
		{name: "missing title", html: scheduleHTML("Year 2026", `<tr><td>release</td></tr>`), want: "release 1 title is required"},
		{name: "invalid supported title", html: scheduleHTML("Year 2026", supportedRow("July 30", "8:30 AM", "GDP (Advance Estimate), annual 2026")), want: "invalid national GDP release title"},
		{name: "missing scheduled date", html: scheduleHTML("Year 2026", `<tr><td><div class="release-date">July 30</div><small>8:30 AM</small></td><td class="release-title">GDP (Advance Estimate), 2nd Quarter 2026</td></tr>`), want: "release time is required"},
		{name: "missing release date", html: scheduleHTML("Year 2026", `<tr><td class="scheduled-date"><small>8:30 AM</small></td><td class="release-title">GDP (Advance Estimate), 2nd Quarter 2026</td></tr>`), want: "release date is required"},
		{name: "missing release time", html: scheduleHTML("Year 2026", supportedRow("July 30", "", "GDP (Advance Estimate), 2nd Quarter 2026")), want: "release time is required"},
		{name: "invalid release time", html: scheduleHTML("Year 2026", supportedRow("July 30", "25:00 PM", "GDP (Advance Estimate), 2nd Quarter 2026")), want: "invalid release date and time"},
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
		config bea.Config
	}{
		{name: "relative URL", config: bea.Config{CalendarURL: "/calendar.html"}},
		{name: "unsupported URL scheme", config: bea.Config{CalendarURL: "file:///calendar.html"}},
		{name: "negative request budget", config: bea.Config{RequestBudget: -time.Second}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := bea.NewAdapter(test.config); err == nil {
				t.Fatal("NewAdapter() error = nil, want validation error")
			}
		})
	}
	if adapter, err := bea.NewAdapter(bea.Config{}); err != nil || adapter == nil {
		t.Fatalf("NewAdapter(defaults) = %v, %v, want configured adapter", adapter, err)
	}
}

func scheduleHTML(heading, rows string) string {
	return `<html><body><table id="release-schedule-table"><thead><tr><th>` + heading +
		`</th></tr></thead><tbody>` + rows + `</tbody></table></body></html>`
}

func supportedRow(date, releaseTime, title string) string {
	timeElement := ""
	if releaseTime != "" {
		timeElement = `<small>` + releaseTime + `</small>`
	}
	return `<tr><td class="scheduled-date"><div class="release-date">` + date + `</div>` + timeElement +
		`</td><td class="release-title">` + title + `</td></tr>`
}

func newAdapter(t *testing.T, client bea.HTTPClient, now func() time.Time, requestBudget time.Duration) *bea.Adapter {
	t.Helper()
	adapter, err := bea.NewAdapter(bea.Config{
		CalendarURL:   "https://example.com/bea.html",
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
