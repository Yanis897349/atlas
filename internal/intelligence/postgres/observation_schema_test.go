package postgres_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/jackc/pgx/v5"
)

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
