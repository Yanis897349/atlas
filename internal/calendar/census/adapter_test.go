package census_test

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
	"github.com/Yanis897349/atlas/internal/calendar/census"
)

func TestAdapterFetchEventsNormalizesRetailSalesReleases(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 11, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	client := fixtureClient(t, "valid.html")
	adapter := newAdapter(t, client, func() time.Time { return retrievedAt }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}

	want := []calendar.Event{
		{
			Source:          census.Source,
			ExternalEventID: "retail-sales-2025-11",
			Name:            "Advance Monthly Sales for Retail and Food Services",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeRetailSales,
			ScheduledAt:     time.Date(2026, time.January, 14, 13, 30, 0, 0, time.UTC),
			SourceURL:       census.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          census.Source,
			ExternalEventID: "retail-sales-2026-05",
			Name:            "Advance Monthly Sales for Retail and Food Services",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeRetailSales,
			ScheduledAt:     time.Date(2026, time.June, 17, 12, 30, 0, 0, time.UTC),
			SourceURL:       census.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
	}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("FetchEvents() = %#v, want %#v", events, want)
	}
	if client.requests != 1 || client.method != http.MethodGet {
		t.Errorf("requests = %d using %s, want one GET", client.requests, client.method)
	}
	if client.accept != "text/html" || client.userAgent != "Atlas (+https://github.com/Yanis897349/atlas)" {
		t.Errorf("request headers = Accept %q, User-Agent %q", client.accept, client.userAgent)
	}
}

func TestAdapterFetchEventsKeepsIdentityAcrossScheduleCorrections(t *testing.T) {
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	client := fixtureClient(t, "valid.html")
	adapter := newAdapter(t, client, func() time.Time { return now }, 0)

	first, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("first FetchEvents() error = %v", err)
	}
	now = now.Add(time.Hour)
	client.contents = bytes.Replace(client.contents, []byte("January 14, 2026"), []byte("January 15, 2026"), 1)
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

func TestAdapterFetchEventsCapturesRetrievalTimeAfterResponse(t *testing.T) {
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	retrievedAt := now.Add(time.Minute)
	client := fixtureClient(t, "valid.html")
	client.onDo = func() { now = retrievedAt }
	adapter := newAdapter(t, client, func() time.Time { return now }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	for _, event := range events {
		if !event.RetrievedAt.Equal(retrievedAt) {
			t.Errorf("RetrievedAt = %v, want post-response time %v", event.RetrievedAt, retrievedAt)
		}
	}
}

func TestAdapterFetchEventsCollapsesRepeatedIdentities(t *testing.T) {
	adapter := newAdapter(t, fixtureClient(t, "repeated.html"), nil, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].ExternalEventID != "retail-sales-2026-07" {
		t.Fatalf("FetchEvents() = %#v, want one July 2026 retail-sales event", events)
	}
	want := time.Date(2026, time.August, 14, 12, 30, 0, 0, time.UTC)
	if !events[0].ScheduledAt.Equal(want) {
		t.Errorf("ScheduledAt = %v, want first-seen time %v", events[0].ScheduledAt, want)
	}
}

func TestAdapterFetchEventsIgnoresUnsupportedAndSuspendedReleases(t *testing.T) {
	for _, fixture := range []string{"unsupported.html", "suspended.html"} {
		t.Run(fixture, func(t *testing.T) {
			adapter := newAdapter(t, fixtureClient(t, fixture), nil, 0)
			events, err := adapter.FetchEvents(t.Context())
			if err != nil {
				t.Fatalf("FetchEvents() error = %v", err)
			}
			if len(events) != 0 {
				t.Errorf("FetchEvents() = %#v, want no events", events)
			}
		})
	}
}

func TestAdapterFetchEventsRejectsMalformedCalendarData(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{name: "invalid fixture period", html: string(fixtureContents(t, "malformed.html")), want: "invalid covered period"},
		{name: "missing table", html: "<html><body>not a calendar</body></html>", want: "calendar table is required"},
		{name: "wrong element type", html: `<div id="calendar"></div>`, want: "calendar table is required"},
		{name: "missing body", html: `<table id="calendar"></table>`, want: "calendar body is required"},
		{name: "empty body", html: `<table id="calendar"><tbody></tbody></table>`, want: "calendar rows are required"},
		{name: "missing indicator", html: calendarHTML(`<tr></tr>`), want: "release 1 indicator is required"},
		{name: "empty indicator", html: calendarHTML(`<tr><td></td></tr>`), want: "release 1 indicator is required"},
		{name: "missing release date", html: calendarHTML(supportedRow("", "", "")), want: "release date is required"},
		{name: "missing remaining cells", html: calendarHTML(`<tr><td>` + supportedTitle + `</td><td>July 16, 2026</td></tr>`), want: "date, time, and covered period are required"},
		{name: "missing release time", html: calendarHTML(supportedRow("July 16, 2026", "", "June 2026")), want: "release time is required"},
		{name: "invalid release date", html: calendarHTML(supportedRow("July 32, 2026", "8:30 AM", "June 2026")), want: "invalid release date and time"},
		{name: "invalid release time", html: calendarHTML(supportedRow("July 16, 2026", "25:00 PM", "June 2026")), want: "invalid release date and time"},
		{name: "missing covered period", html: calendarHTML(supportedRow("July 16, 2026", "8:30 AM", "")), want: "invalid covered period"},
		{name: "invalid covered period", html: calendarHTML(supportedRow("July 16, 2026", "8:30 AM", "Q2 2026")), want: "invalid covered period"},
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

const supportedTitle = "Advance Monthly Sales for Retail and Food Services"

func calendarHTML(rows string) string {
	return `<html><body><table id="calendar"><tbody>` + rows + `</tbody></table></body></html>`
}

func supportedRow(date, releaseTime, period string) string {
	return `<tr><td>` + supportedTitle + `</td><td>` + date + `</td><td>` + releaseTime + `</td><td>` + period + `</td></tr>`
}

func newAdapter(t *testing.T, client census.HTTPClient, now func() time.Time, requestBudget time.Duration) *census.Adapter {
	t.Helper()
	adapter, err := census.NewAdapter(census.Config{
		CalendarURL:   "https://example.com/census.html",
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
	accept    string
	userAgent string
}

func (client *recordingClient) Do(request *http.Request) (*http.Response, error) {
	client.requests++
	client.method = request.Method
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
