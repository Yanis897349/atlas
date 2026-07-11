package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar/fed"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
)

func TestRunIngestsFedCalendarIdempotentlyAndAppliesNewerCorrections(t *testing.T) {
	database := postgrestest.Open(t)
	calendarServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte(testFedCalendar))
	}))
	t.Cleanup(calendarServer.Close)

	retrievedAt := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	stdout := &bytes.Buffer{}
	dependencies := Dependencies{
		Getenv:         applicationDatabaseEnv(database.URL),
		FedHTTPClient:  calendarServer.Client(),
		FedCalendarURL: calendarServer.URL,
		FedNow:         func() time.Time { return retrievedAt },
		Stdout:         stdout,
	}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	for range 2 {
		if err := Run(t.Context(), []string{"ingest-fed"}, dependencies); err != nil {
			t.Fatalf("Run(ingest-fed) error = %v", err)
		}
	}

	var count int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*)
FROM economic_events
WHERE source = 'federal_reserve'
  AND external_event_id IN ('fomc-2026-01-28', 'fomc-2026-07-29')
  AND created_by = 'atlas-fed-calendar-ingestion'
  AND updated_by = 'atlas-fed-calendar-ingestion'
`).Scan(&count); err != nil {
		t.Fatalf("count Federal Reserve events: %v", err)
	}
	if count != 2 {
		t.Fatalf("Federal Reserve event count = %d, want 2", count)
	}

	created := loadFedEvent(t, database)
	if !created.retrievedAt.Equal(retrievedAt) || created.sourceURL != fed.CalendarURL {
		t.Errorf("FOMC source metadata = (%v, %q), want (%v, %q)", created.retrievedAt, created.sourceURL, retrievedAt, fed.CalendarURL)
	}
	if created.createdBy != fedIngestionActor || created.updatedBy != fedIngestionActor {
		t.Errorf("FOMC audit actors = (%q, %q), want %q", created.createdBy, created.updatedBy, fedIngestionActor)
	}

	staleRetrievedAt := retrievedAt.Add(-time.Hour)
	if _, err := database.Pool.Exec(t.Context(), `
UPDATE economic_events
SET name = 'Stale FOMC event',
    scheduled_at = '2026-07-29T17:00:00Z',
    source_url = 'https://example.com/stale',
    retrieved_at = $1
WHERE source = 'federal_reserve' AND external_event_id = 'fomc-2026-07-29'
`, staleRetrievedAt); err != nil {
		t.Fatalf("make FOMC event stale: %v", err)
	}
	retrievedAt = retrievedAt.Add(time.Hour)
	if err := Run(t.Context(), []string{"ingest-fed"}, dependencies); err != nil {
		t.Fatalf("Run(ingest-fed correction) error = %v", err)
	}

	corrected := loadFedEvent(t, database)
	if corrected.id != created.id || !corrected.createdAt.Equal(created.createdAt) || corrected.createdBy != created.createdBy {
		t.Errorf("corrected FOMC identity/creation audit changed from %#v to %#v", created, corrected)
	}
	wantScheduledAt := time.Date(2026, time.July, 29, 18, 0, 0, 0, time.UTC)
	if corrected.name != "Federal Open Market Committee Interest Rate Decision" ||
		!corrected.scheduledAt.Equal(wantScheduledAt) ||
		corrected.sourceURL != fed.CalendarURL ||
		!corrected.retrievedAt.Equal(retrievedAt) {
		t.Errorf("corrected FOMC event = %#v, want official metadata at %v retrieved %v", corrected, wantScheduledAt, retrievedAt)
	}
	if got, want := stdout.String(), strings.Repeat("ingested 2 Federal Reserve calendar events\n", 3); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunReportsFedConfigurationRetrievalAndCancellationFailures(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	t.Run("configuration", func(t *testing.T) {
		dependencies.FedCalendarURL = "://invalid"
		err := Run(t.Context(), []string{"ingest-fed"}, dependencies)
		if err == nil || !strings.Contains(err.Error(), "configure Federal Reserve calendar: invalid Federal Reserve calendar URL") {
			t.Fatalf("Run(ingest-fed) error = %v, want contextual configuration error", err)
		}
	})

	t.Run("HTTP failure", func(t *testing.T) {
		calendarServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
		}))
		t.Cleanup(calendarServer.Close)

		dependencies.FedHTTPClient = calendarServer.Client()
		dependencies.FedCalendarURL = calendarServer.URL
		err := Run(t.Context(), []string{"ingest-fed"}, dependencies)
		if err == nil || !strings.Contains(err.Error(), "ingest Federal Reserve calendar: fetch economic events: fetch Federal Reserve calendar") {
			t.Fatalf("Run(ingest-fed) error = %v, want contextual fetch error", err)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		started := make(chan struct{})
		calendarServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			close(started)
			<-request.Context().Done()
		}))
		t.Cleanup(calendarServer.Close)

		ctx, cancel := context.WithCancel(t.Context())
		dependencies.FedHTTPClient = calendarServer.Client()
		dependencies.FedCalendarURL = calendarServer.URL
		result := make(chan error, 1)
		go func() {
			result <- Run(ctx, []string{"ingest-fed"}, dependencies)
		}()
		<-started
		cancel()

		err := <-result
		if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "ingest Federal Reserve calendar") {
			t.Fatalf("Run(ingest-fed) error = %v, want contextual cancellation", err)
		}
	})
}

type storedFedEvent struct {
	id          string
	name        string
	scheduledAt time.Time
	retrievedAt time.Time
	createdAt   time.Time
	createdBy   string
	updatedBy   string
	sourceURL   string
}

func loadFedEvent(t *testing.T, database postgrestest.Database) storedFedEvent {
	t.Helper()
	var event storedFedEvent
	err := database.Pool.QueryRow(t.Context(), `
SELECT id, name, scheduled_at, retrieved_at, created_at, created_by, updated_by, source_url
FROM economic_events
WHERE source = 'federal_reserve' AND external_event_id = 'fomc-2026-07-29'
`).Scan(
		&event.id,
		&event.name,
		&event.scheduledAt,
		&event.retrievedAt,
		&event.createdAt,
		&event.createdBy,
		&event.updatedBy,
		&event.sourceURL,
	)
	if err != nil {
		t.Fatalf("load FOMC event: %v", err)
	}
	return event
}

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
