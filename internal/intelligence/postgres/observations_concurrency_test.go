package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryEventObservationsLocksEventDuringRetrieval(t *testing.T) {
	pool := openTestPool(t)
	repository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	event := insertEconomicEvent(t, pool, "observation-concurrent-delete")
	created, err := repository.StoreObservation(t.Context(), intelligence.Observation{
		EconomicEventID:     event.ID,
		Source:              "official-statistics",
		SourceObservationID: "concurrent-delete",
		SourceURL:           "https://example.com/releases/concurrent-delete",
		ObservedAt:          time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC),
		Actual:              text("3.2%"),
	}, "observation-ingestion")
	if err != nil {
		t.Fatalf("StoreObservation() error = %v", err)
	}

	blocker, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin observation-table blocker: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback(context.Background()) })
	if _, err := blocker.Exec(t.Context(), `LOCK TABLE economic_event_observations IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock observation table: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	observationsResult := make(chan struct {
		observations []intelligence.StoredObservation
		err          error
	}, 1)
	go func() {
		observations, retrieveErr := repository.EventObservations(ctx, event.ID, 10)
		observationsResult <- struct {
			observations []intelligence.StoredObservation
			err          error
		}{observations: observations, err: retrieveErr}
	}()
	waitForBlockedObservationQuery(t, pool, "FROM economic_event_observations")

	deleteResult := make(chan error, 1)
	go func() {
		_, deleteErr := pool.Exec(ctx, `DELETE FROM economic_events WHERE id = $1`, event.ID)
		deleteResult <- deleteErr
	}()
	waitForBlockedObservationQuery(t, pool, "DELETE FROM economic_events")

	if err := blocker.Commit(t.Context()); err != nil {
		t.Fatalf("release observation-table blocker: %v", err)
	}
	result := <-observationsResult
	if result.err != nil {
		t.Fatalf("EventObservations() error = %v", result.err)
	}
	if len(result.observations) != 1 || result.observations[0].ID != created.ID {
		t.Fatalf("EventObservations() = %#v, want locked observation %q", result.observations, created.ID)
	}
	if err := <-deleteResult; err != nil {
		t.Fatalf("delete economic event: %v", err)
	}
	assertObservationCount(t, pool, 0)
}

func waitForBlockedObservationQuery(t *testing.T, pool *pgxpool.Pool, queryFragment string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		err := pool.QueryRow(t.Context(), `
SELECT EXISTS (
    SELECT 1
    FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
      AND state = 'active'
      AND wait_event_type = 'Lock'
      AND query LIKE '%' || $1 || '%'
)
`, queryFragment).Scan(&blocked)
		if err != nil {
			t.Fatalf("inspect blocked PostgreSQL queries: %v", err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("PostgreSQL query containing %q did not block", queryFragment)
}
