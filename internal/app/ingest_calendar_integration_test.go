package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar/bls"
	"github.com/Yanis897349/atlas/internal/calendar/fed"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
)

func TestRunIngestsCalendarSourcesIdempotently(t *testing.T) {
	tests := calendarCommandTests()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := postgrestest.Open(t)
			server := calendarServer(t, http.StatusOK, test.contentType, test.body)
			retrievedAt := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
			stdout := &bytes.Buffer{}
			dependencies := Dependencies{
				Getenv: applicationDatabaseEnv(database.URL),
				Stdout: stdout,
			}
			test.configure(&dependencies, server, func() time.Time { return retrievedAt })

			if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
				t.Fatalf("Run(migrate) error = %v", err)
			}
			for range 2 {
				if err := Run(t.Context(), []string{test.command}, dependencies); err != nil {
					t.Fatalf("Run(%s) error = %v", test.command, err)
				}
			}

			var count int
			if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*)
FROM economic_events
WHERE source = $1
  AND created_by = $2
  AND updated_by = $2
`, test.source, test.actor).Scan(&count); err != nil {
				t.Fatalf("count %s events: %v", test.name, err)
			}
			if count != test.eventCount {
				t.Fatalf("%s event count = %d, want %d", test.name, count, test.eventCount)
			}

			var sourceURL string
			var storedRetrievedAt time.Time
			if err := database.Pool.QueryRow(t.Context(), `
SELECT source_url, retrieved_at
FROM economic_events
WHERE source = $1
ORDER BY external_event_id
LIMIT 1
`, test.source).Scan(&sourceURL, &storedRetrievedAt); err != nil {
				t.Fatalf("load %s source metadata: %v", test.name, err)
			}
			if sourceURL != test.canonicalURL || !storedRetrievedAt.Equal(retrievedAt) {
				t.Errorf("%s source metadata = (%q, %v), want (%q, %v)", test.name, sourceURL, storedRetrievedAt, test.canonicalURL, retrievedAt)
			}
			if got, want := stdout.String(), strings.Repeat(test.output, 2); got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestRunReportsCalendarConfigurationAndRetrievalFailures(t *testing.T) {
	for _, test := range calendarCommandTests() {
		t.Run(test.name, func(t *testing.T) {
			database := postgrestest.Open(t)
			dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
			if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
				t.Fatalf("Run(migrate) error = %v", err)
			}

			t.Run("configuration", func(t *testing.T) {
				test.configure(&dependencies, nil, nil)
				test.setURL(&dependencies, "://invalid")
				err := Run(t.Context(), []string{test.command}, dependencies)
				if err == nil || !strings.Contains(err.Error(), test.configurationError) {
					t.Fatalf("Run(%s) error = %v, want error containing %q", test.command, err, test.configurationError)
				}
			})

			t.Run("retrieval", func(t *testing.T) {
				server := calendarServer(t, http.StatusServiceUnavailable, "text/plain", "unavailable")
				test.configure(&dependencies, server, nil)
				err := Run(t.Context(), []string{test.command}, dependencies)
				if err == nil || !strings.Contains(err.Error(), test.retrievalError) {
					t.Fatalf("Run(%s) error = %v, want error containing %q", test.command, err, test.retrievalError)
				}
			})
		})
	}
}

type calendarCommandTest struct {
	name               string
	command            string
	source             string
	actor              string
	canonicalURL       string
	contentType        string
	body               string
	eventCount         int
	output             string
	configurationError string
	retrievalError     string
	configure          func(*Dependencies, *httptest.Server, func() time.Time)
	setURL             func(*Dependencies, string)
}

func calendarCommandTests() []calendarCommandTest {
	return []calendarCommandTest{
		{
			name:               "BLS",
			command:            "ingest-bls",
			source:             bls.Source,
			actor:              blsIngestionActor,
			canonicalURL:       bls.CalendarURL,
			contentType:        "text/calendar",
			body:               testBLSCalendar,
			eventCount:         2,
			output:             "ingested 2 BLS calendar events\n",
			configurationError: "configure BLS calendar: invalid BLS calendar URL",
			retrievalError:     "ingest BLS calendar: fetch economic events: fetch BLS calendar",
			configure: func(dependencies *Dependencies, server *httptest.Server, now func() time.Time) {
				dependencies.BLS = calendarSourceDependencies(server, now)
			},
			setURL: func(dependencies *Dependencies, url string) {
				dependencies.BLS.CalendarURL = url
			},
		},
		{
			name:               "Federal Reserve",
			command:            "ingest-fed",
			source:             fed.Source,
			actor:              fedIngestionActor,
			canonicalURL:       fed.CalendarURL,
			contentType:        "text/html",
			body:               testFedCalendar,
			eventCount:         2,
			output:             "ingested 2 Federal Reserve calendar events\n",
			configurationError: "configure Federal Reserve calendar: invalid Federal Reserve calendar URL",
			retrievalError:     "ingest Federal Reserve calendar: fetch economic events: fetch Federal Reserve calendar",
			configure: func(dependencies *Dependencies, server *httptest.Server, now func() time.Time) {
				dependencies.Fed = calendarSourceDependencies(server, now)
			},
			setURL: func(dependencies *Dependencies, url string) {
				dependencies.Fed.CalendarURL = url
			},
		},
	}
}

func calendarSourceDependencies(server *httptest.Server, now func() time.Time) CalendarSourceDependencies {
	dependencies := CalendarSourceDependencies{Now: now}
	if server != nil {
		dependencies.HTTPClient = server.Client()
		dependencies.CalendarURL = server.URL
	}
	return dependencies
}

func calendarServer(t *testing.T, status int, contentType string, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", contentType)
		response.WriteHeader(status)
		_, _ = response.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

const testBLSCalendar = "BEGIN:VCALENDAR\r\n" +
	"VERSION:2.0\r\n" +
	"PRODID:-//Atlas Test//EN\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:cpi-2026-08\r\n" +
	"DTSTART:20260812T123000Z\r\n" +
	"SUMMARY:Consumer Price Index\r\n" +
	"END:VEVENT\r\n" +
	"BEGIN:VEVENT\r\n" +
	"UID:employment-2026-08\r\n" +
	"DTSTART:20260807T123000Z\r\n" +
	"SUMMARY:Employment Situation\r\n" +
	"END:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

const testFedCalendar = `<!doctype html>
<html lang="en"><body>
  <div class="panel panel-default">
    <div class="panel-heading"><h4>2026 FOMC Meetings</h4></div>
    <div class="row fomc-meeting">
      <div class="fomc-meeting__month">January</div>
      <div class="fomc-meeting__date">27-28</div>
    </div>
    <div class="row fomc-meeting">
      <div class="fomc-meeting__month">July</div>
      <div class="fomc-meeting__date">28-29*</div>
    </div>
  </div>
</body></html>`
