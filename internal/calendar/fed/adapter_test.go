package fed_test

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
	"github.com/Yanis897349/atlas/internal/calendar/fed"
)

func TestAdapterFetchEventsNormalizesRegularMeetings(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 11, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	client := fixtureClient(t, "valid.html")
	adapter := newAdapter(t, client, func() time.Time { return retrievedAt }, 0)

	events, err := adapter.FetchEvents(t.Context())
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}

	want := []calendar.Event{
		{
			Source:          fed.Source,
			ExternalEventID: "fomc-2026-01-28",
			Name:            "Federal Open Market Committee Interest Rate Decision",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeInterestRateDecision,
			ScheduledAt:     time.Date(2026, time.January, 28, 19, 0, 0, 0, time.UTC),
			SourceURL:       fed.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          fed.Source,
			ExternalEventID: "fomc-2026-07-29",
			Name:            "Federal Open Market Committee Interest Rate Decision",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeInterestRateDecision,
			ScheduledAt:     time.Date(2026, time.July, 29, 18, 0, 0, 0, time.UTC),
			SourceURL:       fed.CalendarURL,
			RetrievedAt:     retrievedAt.UTC(),
		},
		{
			Source:          fed.Source,
			ExternalEventID: "fomc-2023-02-01",
			Name:            "Federal Open Market Committee Interest Rate Decision",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeInterestRateDecision,
			ScheduledAt:     time.Date(2023, time.February, 1, 19, 0, 0, 0, time.UTC),
			SourceURL:       fed.CalendarURL,
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
	if len(events) != 1 || events[0].ExternalEventID != "fomc-2026-07-29" {
		t.Fatalf("FetchEvents() = %#v, want one July 29 meeting", events)
	}
}

func TestAdapterFetchEventsIgnoresNotationVotesBeforeValidation(t *testing.T) {
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
		{name: "invalid fixture date", html: string(fixtureContents(t, "malformed.html")), want: "invalid meeting end"},
		{name: "missing panels", html: "<html><body>not a calendar</body></html>", want: "meeting panels are required"},
		{name: "invalid year", html: calendarHTML("20XX FOMC Meetings", "January", "27-28"), want: "invalid FOMC year heading"},
		{name: "empty panel", html: calendarHTML("2026 FOMC Meetings", "", ""), want: "has no meeting rows"},
		{name: "missing date", html: meetingHTML("2026 FOMC Meetings", "January", ""), want: "meeting date is required"},
		{name: "missing month", html: meetingHTML("2026 FOMC Meetings", "", "27-28"), want: "meeting month is required"},
		{name: "invalid month", html: meetingHTML("2026 FOMC Meetings", "Smarch", "27-28"), want: "invalid meeting month"},
		{name: "invalid cross month", html: meetingHTML("2026 FOMC Meetings", "January/March", "31-1"), want: "invalid cross-month meeting"},
		{name: "invalid range", html: meetingHTML("2026 FOMC Meetings", "January", "28"), want: "invalid meeting date range"},
		{name: "reversed range", html: meetingHTML("2026 FOMC Meetings", "January", "28-27"), want: "meeting end precedes"},
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
		config fed.Config
	}{
		{name: "relative URL", config: fed.Config{CalendarURL: "/calendar.html"}},
		{name: "unsupported URL scheme", config: fed.Config{CalendarURL: "file:///calendar.html"}},
		{name: "negative request budget", config: fed.Config{RequestBudget: -time.Second}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := fed.NewAdapter(test.config); err == nil {
				t.Fatal("NewAdapter() error = nil, want validation error")
			}
		})
	}
}

func newAdapter(t *testing.T, client fed.HTTPClient, now func() time.Time, requestBudget time.Duration) *fed.Adapter {
	t.Helper()
	adapter, err := fed.NewAdapter(fed.Config{
		CalendarURL:   "https://example.com/fomc.html",
		Client:        client,
		Now:           now,
		RequestBudget: requestBudget,
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	return adapter
}

func calendarHTML(heading, month, date string) string {
	meeting := ""
	if month != "" || date != "" {
		meeting = `<div class="row fomc-meeting"><div class="fomc-meeting__month">` + month +
			`</div><div class="fomc-meeting__date">` + date + `</div></div>`
	}
	return `<html><body><div class="panel"><div class="panel-heading">` + heading + `</div>` + meeting + `</div></body></html>`
}

func meetingHTML(heading, month, date string) string {
	monthElement := ""
	if month != "" {
		monthElement = `<div class="fomc-meeting__month">` + month + `</div>`
	}
	dateElement := ""
	if date != "" {
		dateElement = `<div class="fomc-meeting__date">` + date + `</div>`
	}
	return `<html><body><div class="panel"><div class="panel-heading">` + heading +
		`</div><div class="fomc-meeting">` + monthElement + dateElement + `</div></div></body></html>`
}

type recordingClient struct {
	contents  []byte
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
