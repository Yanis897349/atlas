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

	"github.com/Yanis897349/atlas/internal/calendar/bls"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
)

func TestRunIngestsBLSCalendarIdempotentlyAndAppliesNewerCorrections(t *testing.T) {
	database := postgrestest.Open(t)
	calendarBody := testBLSCalendar("20260812T123000Z")
	calendarServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/calendar")
		_, _ = response.Write([]byte(calendarBody))
	}))
	t.Cleanup(calendarServer.Close)

	retrievedAt := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	stdout := &bytes.Buffer{}
	dependencies := Dependencies{
		Getenv:         applicationDatabaseEnv(database.URL),
		BLSHTTPClient:  calendarServer.Client(),
		BLSCalendarURL: calendarServer.URL,
		BLSNow:         func() time.Time { return retrievedAt },
		Stdout:         stdout,
	}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	for range 2 {
		if err := Run(t.Context(), []string{"ingest-bls"}, dependencies); err != nil {
			t.Fatalf("Run(ingest-bls) error = %v", err)
		}
	}

	var count int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*)
FROM economic_events
WHERE source = 'bls'
  AND external_event_id IN ('cpi-2026-08', 'employment-2026-08')
  AND created_by = 'atlas-bls-calendar-ingestion'
  AND updated_by = 'atlas-bls-calendar-ingestion'
`).Scan(&count); err != nil {
		t.Fatalf("count BLS events: %v", err)
	}
	if count != 2 {
		t.Fatalf("BLS event count = %d, want 2", count)
	}

	created := loadCPIEvent(t, database)
	if !created.retrievedAt.Equal(retrievedAt) || created.sourceURL != bls.CalendarURL {
		t.Errorf("CPI source metadata = (%v, %q), want (%v, %q)", created.retrievedAt, created.sourceURL, retrievedAt, bls.CalendarURL)
	}
	if created.createdBy != blsIngestionActor || created.updatedBy != blsIngestionActor {
		t.Errorf("CPI audit actors = (%q, %q), want %q", created.createdBy, created.updatedBy, blsIngestionActor)
	}

	calendarBody = testBLSCalendar("20260812T133000Z")
	retrievedAt = retrievedAt.Add(time.Hour)
	if err := Run(t.Context(), []string{"ingest-bls"}, dependencies); err != nil {
		t.Fatalf("Run(ingest-bls correction) error = %v", err)
	}

	corrected := loadCPIEvent(t, database)
	if corrected.id != created.id || !corrected.createdAt.Equal(created.createdAt) || corrected.createdBy != created.createdBy {
		t.Errorf("corrected CPI identity/creation audit changed from %#v to %#v", created, corrected)
	}
	wantScheduledAt := time.Date(2026, time.August, 12, 13, 30, 0, 0, time.UTC)
	if !corrected.scheduledAt.Equal(wantScheduledAt) || !corrected.retrievedAt.Equal(retrievedAt) {
		t.Errorf("corrected CPI times = (%v, %v), want (%v, %v)", corrected.scheduledAt, corrected.retrievedAt, wantScheduledAt, retrievedAt)
	}
	if got, want := stdout.String(), strings.Repeat("ingested 2 BLS calendar events\n", 3); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunReportsBLSConfigurationRetrievalAndCancellationFailures(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	t.Run("configuration", func(t *testing.T) {
		dependencies.BLSCalendarURL = "://invalid"
		err := Run(t.Context(), []string{"ingest-bls"}, dependencies)
		if err == nil || !strings.Contains(err.Error(), "configure BLS calendar: invalid BLS calendar URL") {
			t.Fatalf("Run(ingest-bls) error = %v, want contextual configuration error", err)
		}
	})

	t.Run("HTTP failure", func(t *testing.T) {
		calendarServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			http.Error(response, "unavailable", http.StatusServiceUnavailable)
		}))
		t.Cleanup(calendarServer.Close)

		dependencies.BLSHTTPClient = calendarServer.Client()
		dependencies.BLSCalendarURL = calendarServer.URL
		err := Run(t.Context(), []string{"ingest-bls"}, dependencies)
		if err == nil || !strings.Contains(err.Error(), "ingest BLS calendar: fetch economic events: fetch BLS calendar") {
			t.Fatalf("Run(ingest-bls) error = %v, want contextual fetch error", err)
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
		dependencies.BLSHTTPClient = calendarServer.Client()
		dependencies.BLSCalendarURL = calendarServer.URL
		result := make(chan error, 1)
		go func() {
			result <- Run(ctx, []string{"ingest-bls"}, dependencies)
		}()
		<-started
		cancel()

		err := <-result
		if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "ingest BLS calendar") {
			t.Fatalf("Run(ingest-bls) error = %v, want contextual cancellation", err)
		}
	})
}

type storedBLSEvent struct {
	id          string
	scheduledAt time.Time
	retrievedAt time.Time
	createdAt   time.Time
	createdBy   string
	updatedBy   string
	sourceURL   string
}

func loadCPIEvent(t *testing.T, database postgrestest.Database) storedBLSEvent {
	t.Helper()
	var event storedBLSEvent
	err := database.Pool.QueryRow(t.Context(), `
SELECT id, scheduled_at, retrieved_at, created_at, created_by, updated_by, source_url
FROM economic_events
WHERE source = 'bls' AND external_event_id = 'cpi-2026-08'
`).Scan(
		&event.id,
		&event.scheduledAt,
		&event.retrievedAt,
		&event.createdAt,
		&event.createdBy,
		&event.updatedBy,
		&event.sourceURL,
	)
	if err != nil {
		t.Fatalf("load CPI event: %v", err)
	}
	return event
}

func testBLSCalendar(cpiStart string) string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Atlas Test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:cpi-2026-08\r\n" +
		"DTSTART:" + cpiStart + "\r\n" +
		"SUMMARY:Consumer Price Index\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:employment-2026-08\r\n" +
		"DTSTART:20260807T123000Z\r\n" +
		"SUMMARY:Employment Situation\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}
