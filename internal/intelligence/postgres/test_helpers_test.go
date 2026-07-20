package postgres_test

import (
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("apply database migrations: %v", err)
	}
	return database.Pool
}

func insertEconomicEvent(t *testing.T, pool *pgxpool.Pool, identity string) calendar.StoredEvent {
	t.Helper()
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository(calendar) error = %v", err)
	}
	stored, err := repository.UpsertEvent(t.Context(), calendar.Event{
		Source:          "official-calendar",
		ExternalEventID: identity,
		Name:            "Economic event " + identity,
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeInflation,
		ScheduledAt:     time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC),
		SourceURL:       "https://example.com/calendar/" + identity,
		RetrievedAt:     time.Date(2026, time.July, 15, 8, 0, 0, 0, time.UTC),
	}, "calendar-ingestion")
	if err != nil {
		t.Fatalf("UpsertEvent(%q) error = %v", identity, err)
	}
	return stored
}

func observationFixture(
	eventID string,
	identity string,
	observedAt time.Time,
	consensus *string,
	previous *string,
	actual *string,
) intelligence.Observation {
	return intelligence.Observation{
		EconomicEventID:     eventID,
		Source:              "official-statistics",
		SourceObservationID: identity,
		SourceURL:           "https://example.com/releases/" + identity,
		ObservedAt:          observedAt,
		Consensus:           consensus,
		Previous:            previous,
		Actual:              actual,
	}
}

func text(value string) *string {
	return &value
}

func value(input *string) string {
	if input == nil {
		return ""
	}
	return *input
}

func assertUTCObservation(t *testing.T, observation intelligence.StoredObservation) {
	t.Helper()
	for name, timestamp := range map[string]time.Time{
		"observed": observation.ObservedAt,
		"created":  observation.CreatedAt,
		"updated":  observation.UpdatedAt,
	} {
		if timestamp.Location() != time.UTC {
			t.Errorf("%s location = %v, want UTC", name, timestamp.Location())
		}
	}
}

func assertObservationCount(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM economic_event_observations`).Scan(&count); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if count != want {
		t.Errorf("observation count = %d, want %d", count, want)
	}
}

func assertObservationWatermarkCount(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM economic_event_observation_watermarks`).Scan(&count); err != nil {
		t.Fatalf("count observation watermarks: %v", err)
	}
	if count != want {
		t.Errorf("observation watermark count = %d, want %d", count, want)
	}
}
