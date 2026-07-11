package spglobal_test

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
	"github.com/Yanis897349/atlas/internal/calendar/spglobal"
)

func TestAdapterFetchEventsNormalizesFlashEurozoneReleases(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 11, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	client := fixtureClient(t, "valid.html")
	adapter := newAdapter(t, client, func() time.Time { return retrievedAt }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}

	want := []calendar.Event{
		{
			Source:          spglobal.Source,
			ExternalEventID: "eurozone-flash-pmi-2026-01",
			Name:            "S&P Global Flash Eurozone PMI",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypePMI,
			ScheduledAt:     time.Date(2026, time.January, 23, 8, 0, 0, 0, time.UTC),
			SourceURL:       spglobal.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          spglobal.Source,
			ExternalEventID: "eurozone-flash-pmi-2026-06",
			Name:            "S&P Global Flash Eurozone PMI",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypePMI,
			ScheduledAt:     time.Date(2026, time.June, 23, 8, 0, 0, 0, time.UTC),
			SourceURL:       spglobal.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          spglobal.Source,
			ExternalEventID: "eurozone-flash-pmi-2027-01",
			Name:            "S&P Global Flash Eurozone PMI",
			Region:          calendar.RegionEurozone,
			Type:            calendar.EventTypePMI,
			ScheduledAt:     time.Date(2027, time.January, 22, 8, 0, 0, 0, time.UTC),
			SourceURL:       spglobal.CalendarURL,
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
	client.contents = bytes.Replace(client.contents, []byte("January 23"), []byte("January 24"), 1)
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
	if len(events) != 1 || events[0].ExternalEventID != "eurozone-flash-pmi-2026-07" {
		t.Fatalf("FetchEvents() = %#v, want one July 2026 release", events)
	}
	want := time.Date(2026, time.July, 23, 8, 0, 0, 0, time.UTC)
	if !events[0].ScheduledAt.Equal(want) {
		t.Errorf("ScheduledAt = %v, want first-seen time %v", events[0].ScheduledAt, want)
	}
}

func TestAdapterFetchEventsIgnoresUnsupportedReleasesBeforeValidation(t *testing.T) {
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
		{name: "invalid fixture time", html: string(fixtureContents(t, "malformed.html")), want: "invalid UTC release date and time"},
		{name: "release before year", html: calendarHTML("", "", supportedRelease("08:00")+`<div>2026</div><div>June 23</div><div>08:30 UTC Other PMI</div>`), want: "release year is required"},
		{name: "release before date", html: calendarHTML("2026", "", supportedRelease("08:00")+`<div>June 23</div><div>08:30 UTC Other PMI</div>`), want: "release date is required"},
		{name: "missing time", html: calendarHTML("2026", "June 23", `<div>`+supportedTitle+`</div>`), want: "release time is required"},
		{name: "invalid month", html: calendarHTML("2026", "Smarch 23", supportedRelease("08:00")), want: "invalid UTC release date and time"},
		{name: "invalid day", html: calendarHTML("2026", "June 32", supportedRelease("08:00")), want: "invalid UTC release date and time"},
		{name: "invalid time", html: calendarHTML("2026", "June 23", supportedRelease("8am")), want: "invalid UTC release date and time"},
		{name: "missing calendar year", html: `<html><body><div>June 23</div><div>08:00 UTC Other PMI</div></body></html>`, want: "calendar year is required"},
		{name: "missing calendar date", html: `<html><body><div>2026</div><div>08:00 UTC Other PMI</div></body></html>`, want: "calendar date is required"},
		{name: "missing calendar releases", html: `<html><body><div>2026</div><div>June 23</div></body></html>`, want: "calendar releases are required"},
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

const supportedTitle = "S&P Global Flash Eurozone PMI"

func calendarHTML(year, date, releases string) string {
	return `<html><body><main><div>` + year + `</div><div>` + date + `</div>` + releases + `</main></body></html>`
}

func supportedRelease(releaseTime string) string {
	return `<div>` + releaseTime + ` UTC ` + supportedTitle + `</div>`
}

func newAdapter(t *testing.T, client spglobal.HTTPClient, now func() time.Time, requestBudget time.Duration) *spglobal.Adapter {
	t.Helper()
	adapter, err := spglobal.NewAdapter(spglobal.Config{
		CalendarURL:   "https://example.com/calendar.html",
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
