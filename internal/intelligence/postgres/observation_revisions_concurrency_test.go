package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
)

func TestRepositoryObservationRevisionsLocksEventDuringRetrieval(t *testing.T) {
	pool := openTestPool(t)
	repository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	event := insertEconomicEvent(t, pool, "observation-revision-concurrent-delete")
	created, err := repository.StoreObservation(t.Context(), intelligence.Observation{
		EconomicEventID:     event.ID,
		Source:              "official-statistics",
		SourceObservationID: "revision-concurrent-delete",
		SourceURL:           "https://example.com/releases/revision-concurrent-delete",
		ObservedAt:          time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC),
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
	revisionsResult := make(chan struct {
		revisions []intelligence.StoredObservation
		err       error
	}, 1)
	go func() {
		revisions, retrieveErr := repository.ObservationRevisions(
			ctx,
			event.ID,
			"official-statistics",
			"revision-concurrent-delete",
			10,
		)
		revisionsResult <- struct {
			revisions []intelligence.StoredObservation
			err       error
		}{revisions: revisions, err: retrieveErr}
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
	result := <-revisionsResult
	if result.err != nil {
		t.Fatalf("ObservationRevisions() error = %v", result.err)
	}
	if len(result.revisions) != 1 || result.revisions[0].ID != created.ID {
		t.Fatalf("ObservationRevisions() = %#v, want locked revision %q", result.revisions, created.ID)
	}
	if err := <-deleteResult; err != nil {
		t.Fatalf("delete economic event: %v", err)
	}
	assertObservationCount(t, pool, 0)
}
