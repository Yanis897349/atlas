package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryEventLinksLocksInstrumentDuringRetrieval(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Concurrent update", Symbols: []string{"SPY"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}
	eventID := "00000000-0000-0000-0000-000000000701"
	insertEconomicEvent(t, database.Pool, eventID, "concurrent-update", time.Now())
	createdLink, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, "SPY", eventID, "analyst",
	)
	if err != nil {
		t.Fatalf("CreateEventLink() error = %v", err)
	}

	blocker, err := database.Pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin table-lock transaction: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback(context.Background()) })
	if _, err := blocker.Exec(t.Context(), `LOCK TABLE watchlist_event_links IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock event-link table: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	linksResult := make(chan struct {
		links []watchlist.StoredEventLink
		err   error
	}, 1)
	go func() {
		links, retrieveErr := repository.EventLinks(ctx, storedWatchlist.ID, "SPY", 10)
		linksResult <- struct {
			links []watchlist.StoredEventLink
			err   error
		}{links: links, err: retrieveErr}
	}()
	waitForBlockedQuery(t, database.Pool, "FROM watchlist_event_links AS link")

	updateResult := make(chan error, 1)
	go func() {
		_, updateErr := repository.UpdateWatchlist(ctx, storedWatchlist.ID, watchlist.Definition{
			Name: "Concurrent update", Symbols: []string{"SPY"},
		}, "editor")
		updateResult <- updateErr
	}()
	waitForBlockedQuery(t, database.Pool, "DELETE FROM watchlist_instruments")

	if err := blocker.Commit(t.Context()); err != nil {
		t.Fatalf("release event-link table lock: %v", err)
	}

	result := <-linksResult
	if result.err != nil {
		t.Fatalf("EventLinks() error = %v", result.err)
	}
	if len(result.links) != 1 || result.links[0].ID != createdLink.ID {
		t.Fatalf("EventLinks() = %#v, want original link %s", result.links, createdLink.ID)
	}
	if err := <-updateResult; err != nil {
		t.Fatalf("UpdateWatchlist() error = %v", err)
	}
}

func waitForBlockedQuery(t *testing.T, pool *pgxpool.Pool, queryFragment string) {
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
