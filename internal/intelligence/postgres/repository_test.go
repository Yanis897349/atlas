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

func TestRepositoryStoresImmutableObservationRevisions(t *testing.T) {
	pool := openTestPool(t)
	repository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	event := insertEconomicEvent(t, pool, "observation-store")
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

	created, err := repository.StoreObservation(t.Context(), initial, " observation-ingestion ")
	if err != nil {
		t.Fatalf("first StoreObservation() error = %v", err)
	}
	if created.EconomicEventID != event.ID || created.Source != "official-statistics" ||
		created.SourceObservationID != "cpi-2026-06" || value(created.Consensus) != "3.2%" ||
		value(created.Previous) != "3.1%" || created.Actual != nil ||
		created.CreatedBy != "observation-ingestion" || created.UpdatedBy != "observation-ingestion" {
		t.Errorf("created observation = %#v, want normalized exact snapshot", created)
	}
	assertUTCObservation(t, created)

	retried, err := repository.StoreObservation(t.Context(), initial, "retry-worker")
	if err != nil {
		t.Fatalf("retry StoreObservation() error = %v", err)
	}
	if !reflect.DeepEqual(retried, created) {
		t.Errorf("retried observation = %#v, want unchanged %#v", retried, created)
	}
	assertObservationCount(t, pool, 1)

	laterUnchanged := initial
	laterUnchanged.ObservedAt = initial.ObservedAt.Add(time.Hour)
	unchanged, err := repository.StoreObservation(t.Context(), laterUnchanged, "later-worker")
	if err != nil {
		t.Fatalf("later unchanged StoreObservation() error = %v", err)
	}
	if !reflect.DeepEqual(unchanged, created) {
		t.Errorf("later unchanged observation = %#v, want original revision %#v", unchanged, created)
	}
	assertObservationCount(t, pool, 1)

	delayedOlder := initial
	delayedOlder.Source = "official-statistics"
	delayedOlder.SourceObservationID = "cpi-2026-06"
	delayedOlder.ObservedAt = initial.ObservedAt.Add(30 * time.Minute)
	delayedOlder.Actual = text("stale")
	unchanged, err = repository.StoreObservation(t.Context(), delayedOlder, "older-worker")
	if err != nil {
		t.Fatalf("delayed older StoreObservation() error = %v", err)
	}
	if !reflect.DeepEqual(unchanged, created) {
		t.Errorf("delayed older observation = %#v, want unchanged %#v", unchanged, created)
	}

	citationChange := initial
	citationChange.SourceURL = "https://example.com/releases/cpi-2026-06-revised"
	citationChange.ObservedAt = initial.ObservedAt.Add(2 * time.Hour)
	citationRevision, err := repository.StoreObservation(t.Context(), citationChange, "citation-worker")
	if err != nil {
		t.Fatalf("citation revision StoreObservation() error = %v", err)
	}
	if citationRevision.ID == created.ID || citationRevision.SourceURL != citationChange.SourceURL ||
		citationRevision.CreatedBy != "citation-worker" || citationRevision.UpdatedBy != "citation-worker" {
		t.Errorf("citation revision = %#v, want distinct immutable revision", citationRevision)
	}
	assertUTCObservation(t, citationRevision)

	newer := citationChange
	newer.Source = "official-statistics"
	newer.SourceObservationID = "cpi-2026-06"
	newer.ObservedAt = initial.ObservedAt.Add(3 * time.Hour)
	newer.Consensus = nil
	newer.Previous = text("3.0%")
	newer.Actual = text("3.3%")
	revised, err := repository.StoreObservation(t.Context(), newer, "refresh-worker")
	if err != nil {
		t.Fatalf("newer StoreObservation() error = %v", err)
	}
	if revised.ID == created.ID || revised.ID == citationRevision.ID {
		t.Errorf("newer observation ID = %q, want a new revision", revised.ID)
	}
	if revised.Consensus != nil || value(revised.Previous) != "3.0%" || value(revised.Actual) != "3.3%" ||
		revised.SourceURL != newer.SourceURL || !revised.ObservedAt.Equal(newer.ObservedAt) ||
		revised.CreatedBy != "refresh-worker" || revised.UpdatedBy != "refresh-worker" {
		t.Errorf("newer observation = %#v, want immutable changed revision", revised)
	}
	assertUTCObservation(t, revised)
	assertObservationCount(t, pool, 3)

	var (
		originalURL, originalPrevious string
		originalConsensus             *string
		originalActual                *string
		originalCreatedBy             string
		originalUpdatedBy             string
	)
	if err := pool.QueryRow(t.Context(), `
SELECT source_url, consensus_value, previous_value, actual_value, created_by, updated_by
FROM economic_event_observations
WHERE id = $1
`, created.ID).Scan(
		&originalURL,
		&originalConsensus,
		&originalPrevious,
		&originalActual,
		&originalCreatedBy,
		&originalUpdatedBy,
	); err != nil {
		t.Fatalf("load original observation revision: %v", err)
	}
	if originalURL != created.SourceURL || value(originalConsensus) != "3.2%" ||
		originalPrevious != "3.1%" || originalActual != nil ||
		originalCreatedBy != "observation-ingestion" || originalUpdatedBy != "observation-ingestion" {
		t.Errorf("original observation revision changed after later revisions")
	}
}

func TestRepositoryComparesObservationTimesAtPostgreSQLPrecision(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := intelligencepostgres.NewRepository(pool)
	event := insertEconomicEvent(t, pool, "observation-timestamp-precision")
	base := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
	initial := observationFixture(
		event.ID,
		"precision",
		base.Add(100*time.Nanosecond),
		nil,
		nil,
		text("3.0%"),
	)
	created, err := repository.StoreObservation(t.Context(), initial, "worker")
	if err != nil {
		t.Fatalf("StoreObservation(initial) error = %v", err)
	}
	if !created.ObservedAt.Equal(base) {
		t.Fatalf("initial observed at = %s, want PostgreSQL precision %s", created.ObservedAt, base)
	}

	equalAtDatabasePrecision := initial
	equalAtDatabasePrecision.ObservedAt = base.Add(900 * time.Nanosecond)
	equalAtDatabasePrecision.Actual = text("3.1%")
	unchanged, err := repository.StoreObservation(t.Context(), equalAtDatabasePrecision, "equal-worker")
	if err != nil {
		t.Fatalf("StoreObservation(equal PostgreSQL timestamp) error = %v", err)
	}
	if !reflect.DeepEqual(unchanged, created) {
		t.Errorf("equal PostgreSQL timestamp observation = %#v, want unchanged %#v", unchanged, created)
	}
	assertObservationCount(t, pool, 1)

	newer := equalAtDatabasePrecision
	newer.ObservedAt = base.Add(time.Microsecond + 100*time.Nanosecond)
	revised, err := repository.StoreObservation(t.Context(), newer, "revision-worker")
	if err != nil {
		t.Fatalf("StoreObservation(newer PostgreSQL timestamp) error = %v", err)
	}
	if revised.ID == created.ID || !revised.ObservedAt.Equal(base.Add(time.Microsecond)) {
		t.Errorf("newer PostgreSQL timestamp revision = %#v, want distinct microsecond revision", revised)
	}
	assertObservationCount(t, pool, 2)
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
		result, err := repository.StoreObservation(t.Context(), input, "observation-ingestion")
		if err != nil {
			t.Fatalf("StoreObservation(%q) error = %v", input.SourceObservationID, err)
		}
		stored[input.SourceObservationID] = result
	}
	revisedOlder := inputs[0]
	revisedOlder.ObservedAt = base.Add(2 * time.Hour)
	revisedOlder.Actual = text("3.0%")
	latestOlder, err := repository.StoreObservation(t.Context(), revisedOlder, "revision-worker")
	if err != nil {
		t.Fatalf("StoreObservation(revised older) error = %v", err)
	}

	latestIDs := []string{stored["latest-a"].ID, stored["latest-b"].ID}
	sort.Strings(latestIDs)
	got, err := repository.EventObservations(t.Context(), strings.ToUpper(event.ID), 2)
	if err != nil {
		t.Fatalf("EventObservations() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != latestOlder.ID || got[1].ID != latestIDs[0] {
		t.Fatalf("EventObservations() = %#v, want latest revision then smallest tied UUID", got)
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
	if len(all) != 3 || all[0].ID != latestOlder.ID ||
		all[1].ID != latestIDs[0] || all[2].ID != latestIDs[1] {
		t.Errorf("EventObservations(all) = %#v, want only latest identity revisions in deterministic order", all)
	}
	for _, observation := range all {
		if observation.ID == stored["older"].ID {
			t.Errorf("EventObservations(all) included superseded revision %q", observation.ID)
		}
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
	if _, err := repository.StoreObservation(t.Context(), missing, "worker"); err == nil ||
		!errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "lock economic event for observation storage") {
		t.Fatalf("StoreObservation(missing event) error = %v, want contextual reference failure", err)
	}

	event := insertEconomicEvent(t, pool, "observation-cascade")
	initial := observationFixture(
		event.ID, "cascade", time.Now(), nil, nil, text("3.0%"),
	)
	if _, err := repository.StoreObservation(t.Context(), initial, "worker"); err != nil {
		t.Fatalf("StoreObservation() error = %v", err)
	}
	revision := initial
	revision.ObservedAt = revision.ObservedAt.Add(time.Minute)
	revision.Actual = text("3.1%")
	if _, err := repository.StoreObservation(t.Context(), revision, "revision-worker"); err != nil {
		t.Fatalf("StoreObservation(revision) error = %v", err)
	}
	assertObservationCount(t, pool, 2)
	assertObservationWatermarkCount(t, pool, 1)
	if _, err := pool.Exec(t.Context(), `DELETE FROM economic_events WHERE id = $1`, event.ID); err != nil {
		t.Fatalf("delete economic event: %v", err)
	}
	assertObservationCount(t, pool, 0)
	assertObservationWatermarkCount(t, pool, 0)
}

func TestObservationSchemaRejectsDuplicateRevisionTimestamp(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := intelligencepostgres.NewRepository(pool)
	event := insertEconomicEvent(t, pool, "observation-revision-identity")
	observation := observationFixture(
		event.ID,
		"same-revision",
		time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC),
		nil,
		nil,
		text("3.0%"),
	)
	if _, err := repository.StoreObservation(t.Context(), observation, "worker"); err != nil {
		t.Fatalf("StoreObservation() error = %v", err)
	}
	_, err := pool.Exec(t.Context(), `
INSERT INTO economic_event_observations (
    economic_event_id, source, source_observation_id, source_url, observed_at,
    actual_value, created_by, updated_by
) VALUES ($1, $2, $3, $4, $5, '3.1%', 'direct-worker', 'direct-worker')
`, observation.EconomicEventID, observation.Source, observation.SourceObservationID,
		observation.SourceURL, observation.ObservedAt)
	if err == nil {
		t.Fatal("duplicate observation revision timestamp insert succeeded")
	}
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
