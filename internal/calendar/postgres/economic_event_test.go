package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryEconomicEventReturnsCompleteCanonicalEventByUUID(t *testing.T) {
	pool := openTestPool(t)
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	location := time.FixedZone("CEST", 2*60*60)
	input := calendar.Event{
		Source:          "official-calendar",
		ExternalEventID: "event-context",
		Name:            "Consumer Price Index",
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeInflation,
		ScheduledAt:     time.Date(2026, time.August, 12, 14, 30, 0, 0, location),
		SourceURL:       "https://example.com/calendar/event-context",
		RetrievedAt:     time.Date(2026, time.July, 14, 10, 0, 0, 0, location),
	}
	stored, err := repository.UpsertEvent(t.Context(), input, "calendar-ingestion")
	if err != nil {
		t.Fatalf("UpsertEvent() error = %v", err)
	}

	got, err := repository.EconomicEvent(t.Context(), strings.ToUpper(stored.ID))
	if err != nil {
		t.Fatalf("EconomicEvent() error = %v", err)
	}
	if !reflect.DeepEqual(got, stored) {
		t.Errorf("EconomicEvent() = %#v, want %#v", got, stored)
	}
	if got.Source == "" || got.SourceURL == "" {
		t.Errorf("EconomicEvent() source citation = (%q, %q), want complete citation", got.Source, got.SourceURL)
	}
	for name, value := range map[string]time.Time{
		"scheduled": got.ScheduledAt,
		"retrieved": got.RetrievedAt,
		"created":   got.CreatedAt,
		"updated":   got.UpdatedAt,
	} {
		if value.Location() != time.UTC {
			t.Errorf("EconomicEvent() %s location = %v, want UTC", name, value.Location())
		}
	}
}

func TestRepositoryEconomicEventReturnsNotFound(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := calendarpostgres.NewRepository(pool)

	got, err := repository.EconomicEvent(t.Context(), "00000000-0000-0000-0000-000000000083")
	if !errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "query economic event") {
		t.Fatalf("EconomicEvent() error = %v, want contextual pgx.ErrNoRows", err)
	}
	if !reflect.DeepEqual(got, calendar.StoredEvent{}) {
		t.Errorf("EconomicEvent() = %#v, want zero event", got)
	}
}

func TestRepositoryEconomicEventValidatesUUIDBeforePostgreSQL(t *testing.T) {
	repository, err := calendarpostgres.NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	for _, id := range []string{"", "not-a-uuid", "00000000X0000X0000X0000X000000000083"} {
		got, queryErr := repository.EconomicEvent(t.Context(), id)
		if queryErr == nil || !strings.Contains(queryErr.Error(), "event ID must be a UUID") {
			t.Fatalf("EconomicEvent(%q) error = %v, want UUID validation", id, queryErr)
		}
		if !reflect.DeepEqual(got, calendar.StoredEvent{}) {
			t.Errorf("EconomicEvent(%q) = %#v, want zero event", id, got)
		}
	}
}

func TestRepositoryEconomicEventPreservesQueryFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "database", err: errors.New("database unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, err := calendarpostgres.NewRepository(economicEventFailureDB{err: test.err})
			if err != nil {
				t.Fatalf("NewRepository() error = %v", err)
			}
			got, err := repository.EconomicEvent(
				t.Context(),
				"00000000-0000-0000-0000-000000000083",
			)
			if !errors.Is(err, test.err) || !strings.Contains(err.Error(), "query economic event") {
				t.Fatalf("EconomicEvent() error = %v, want contextual %v", err, test.err)
			}
			if !reflect.DeepEqual(got, calendar.StoredEvent{}) {
				t.Errorf("EconomicEvent() = %#v, want zero event", got)
			}
		})
	}
}

type economicEventFailureDB struct {
	err error
}

func (database economicEventFailureDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("EconomicEvent must use QueryRow")
}

func (database economicEventFailureDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return economicEventFailureRow(database)
}

type economicEventFailureRow struct {
	err error
}

func (row economicEventFailureRow) Scan(...any) error {
	return row.err
}
