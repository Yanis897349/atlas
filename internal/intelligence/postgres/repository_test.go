package postgres_test

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryUpsertsLatestObservationSnapshot(t *testing.T) {
	pool := openTestPool(t)
	repository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	event := insertEconomicEvent(t, pool, "observation-upsert")
	location := time.FixedZone("CEST", 2*60*60)
	initial := intelligence.Observation{
		EconomicEventID:     strings.ToUpper(event.ID),
		Source:              " official-statistics ",
		SourceObservationID: " cpi-2026-06 ",
		SourceURL:           "https://example.com/releases/cpi-2026-06",
		ObservedAt:          time.Date(2026, time.July, 15, 14, 0, 0, 0, location),
		Consensus:           text(" 3.2% "),
		Previous:            text("3.1%"),
	}

	created, err := repository.UpsertObservation(t.Context(), initial, " observation-ingestion ")
	if err != nil {
		t.Fatalf("first UpsertObservation() error = %v", err)
	}
	if created.EconomicEventID != event.ID || created.Source != "official-statistics" ||
		created.SourceObservationID != "cpi-2026-06" || value(created.Consensus) != "3.2%" ||
		value(created.Previous) != "3.1%" || created.Actual != nil ||
		created.CreatedBy != "observation-ingestion" || created.UpdatedBy != "observation-ingestion" {
		t.Errorf("created observation = %#v, want normalized exact snapshot", created)
	}
	assertUTCObservation(t, created)

	retried, err := repository.UpsertObservation(t.Context(), initial, "retry-worker")
	if err != nil {
		t.Fatalf("retry UpsertObservation() error = %v", err)
	}
	if !reflect.DeepEqual(retried, created) {
		t.Errorf("retried observation = %#v, want unchanged %#v", retried, created)
	}

	older := initial
	older.Source = "official-statistics"
	older.SourceObservationID = "cpi-2026-06"
	older.ObservedAt = initial.ObservedAt.Add(-time.Minute)
	older.Actual = text("stale")
	unchanged, err := repository.UpsertObservation(t.Context(), older, "older-worker")
	if err != nil {
		t.Fatalf("older UpsertObservation() error = %v", err)
	}
	if !reflect.DeepEqual(unchanged, created) {
		t.Errorf("older observation = %#v, want unchanged %#v", unchanged, created)
	}

	if _, err := pool.Exec(t.Context(), `SELECT pg_sleep(0.01)`); err != nil {
		t.Fatalf("wait before observation update: %v", err)
	}
	newer := initial
	newer.Source = "official-statistics"
	newer.SourceObservationID = "cpi-2026-06"
	newer.SourceURL = "https://example.com/releases/cpi-2026-06-final"
	newer.ObservedAt = initial.ObservedAt.Add(time.Hour)
	newer.Consensus = nil
	newer.Previous = text("3.0%")
	newer.Actual = text("3.3%")
	updated, err := repository.UpsertObservation(t.Context(), newer, "refresh-worker")
	if err != nil {
		t.Fatalf("newer UpsertObservation() error = %v", err)
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt || updated.CreatedBy != created.CreatedBy {
		t.Errorf("creation metadata changed from %#v to %#v", created, updated)
	}
	if updated.Consensus != nil || value(updated.Previous) != "3.0%" || value(updated.Actual) != "3.3%" ||
		updated.SourceURL != newer.SourceURL || !updated.ObservedAt.Equal(newer.ObservedAt) ||
		!updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedBy != "refresh-worker" {
		t.Errorf("updated observation = %#v, want newer replacement snapshot", updated)
	}
	assertObservationCount(t, pool, 1)
}

func TestRepositoryRetrievesObservationsDeterministically(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := intelligencepostgres.NewRepository(pool)
	event := insertEconomicEvent(t, pool, "observation-query")
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	inputs := []intelligence.Observation{
		observationFixture(event.ID, "older", base, nil, nil, text("2.9%")),
		observationFixture(event.ID, "latest-b", base.Add(time.Hour), text("3.1%"), nil, nil),
		observationFixture(event.ID, "latest-a", base.Add(time.Hour), nil, text("3.0%"), text("3.2%")),
	}
	stored := make(map[string]intelligence.StoredObservation, len(inputs))
	for _, input := range inputs {
		result, err := repository.UpsertObservation(t.Context(), input, "observation-ingestion")
		if err != nil {
			t.Fatalf("UpsertObservation(%q) error = %v", input.SourceObservationID, err)
		}
		stored[input.SourceObservationID] = result
	}

	latestIDs := []string{stored["latest-a"].ID, stored["latest-b"].ID}
	sort.Strings(latestIDs)
	got, err := repository.EventObservations(t.Context(), strings.ToUpper(event.ID), 2)
	if err != nil {
		t.Fatalf("EventObservations() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != latestIDs[0] || got[1].ID != latestIDs[1] {
		t.Fatalf("EventObservations() = %#v, want latest tie ordered by UUID %v", got, latestIDs)
	}
	for _, observation := range got {
		if observation.Source == "" || observation.SourceURL == "" || observation.CreatedBy == "" {
			t.Errorf("observation = %#v, want complete source and audit metadata", observation)
		}
		assertUTCObservation(t, observation)
	}

	all, err := repository.EventObservations(t.Context(), event.ID, 3)
	if err != nil {
		t.Fatalf("EventObservations(all) error = %v", err)
	}
	if len(all) != 3 || all[2].ID != stored["older"].ID {
		t.Errorf("EventObservations(all) = %#v, want older observation last", all)
	}

	emptyEvent := insertEconomicEvent(t, pool, "observation-empty")
	empty, err := repository.EventObservations(t.Context(), emptyEvent.ID, 10)
	if err != nil {
		t.Fatalf("EventObservations(empty) error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("EventObservations(empty) = %#v, want non-nil empty slice", empty)
	}

	missing, err := repository.EventObservations(
		t.Context(),
		"00000000-0000-0000-0000-000000000999",
		10,
	)
	if !errors.Is(err, pgx.ErrNoRows) || missing != nil {
		t.Errorf("EventObservations(missing) = (%#v, %v), want nil and pgx.ErrNoRows", missing, err)
	}
}

func TestRepositoryEnforcesObservationReferencesAndCascade(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := intelligencepostgres.NewRepository(pool)
	missing := observationFixture(
		"00000000-0000-0000-0000-000000000999",
		"missing-event",
		time.Now(),
		text("3.0%"),
		nil,
		nil,
	)
	if _, err := repository.UpsertObservation(t.Context(), missing, "worker"); err == nil || !strings.Contains(err.Error(), "upsert economic event observation") {
		t.Fatalf("UpsertObservation(missing event) error = %v, want contextual reference failure", err)
	}

	event := insertEconomicEvent(t, pool, "observation-cascade")
	if _, err := repository.UpsertObservation(t.Context(), observationFixture(
		event.ID, "cascade", time.Now(), nil, nil, text("3.0%"),
	), "worker"); err != nil {
		t.Fatalf("UpsertObservation() error = %v", err)
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM economic_events WHERE id = $1`, event.ID); err != nil {
		t.Fatalf("delete economic event: %v", err)
	}
	assertObservationCount(t, pool, 0)
}

func TestObservationSchemaRequiresNonblankValues(t *testing.T) {
	pool := openTestPool(t)
	event := insertEconomicEvent(t, pool, "observation-constraints")
	for _, test := range []struct {
		name      string
		consensus any
		previous  any
		actual    any
	}{
		{name: "missing values"},
		{name: "blank consensus", consensus: " "},
		{name: "blank previous", previous: "\t"},
		{name: "blank actual", actual: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), `
INSERT INTO economic_event_observations (
    economic_event_id, source, source_observation_id, source_url, observed_at,
    consensus_value, previous_value, actual_value, created_by, updated_by
) VALUES ($1, 'source', $2, 'https://example.com/release', statement_timestamp(), $3, $4, $5, 'worker', 'worker')
`, event.ID, test.name, test.consensus, test.previous, test.actual)
			if err == nil {
				t.Fatal("invalid direct observation insert succeeded")
			}
		})
	}
}

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
