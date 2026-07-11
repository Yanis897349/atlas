package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryDeletesOneEventLinkAndPreservesReferences(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Selective deletion", Symbols: []string{"SPY", "EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}

	deletedEventID := "00000000-0000-0000-0000-000000000801"
	retainedEventID := "00000000-0000-0000-0000-000000000802"
	insertEconomicEvent(t, database.Pool, deletedEventID, "deleted-link", time.Now())
	insertEconomicEvent(t, database.Pool, retainedEventID, "retained-link", time.Now().Add(time.Hour))
	deletedLink, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, "SPY", deletedEventID, "analyst",
	)
	if err != nil {
		t.Fatalf("CreateEventLink(deleted) error = %v", err)
	}
	retainedLink, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, "SPY", retainedEventID, "analyst",
	)
	if err != nil {
		t.Fatalf("CreateEventLink(retained) error = %v", err)
	}

	if err := repository.DeleteEventLink(
		t.Context(), storedWatchlist.ID, " spy ", deletedEventID,
	); err != nil {
		t.Fatalf("DeleteEventLink() error = %v", err)
	}

	links, err := repository.EventLinks(t.Context(), storedWatchlist.ID, "SPY", 10)
	if err != nil {
		t.Fatalf("EventLinks() error = %v", err)
	}
	if !reflect.DeepEqual(links, []watchlist.StoredEventLink{retainedLink}) {
		t.Errorf("EventLinks() = %#v, want retained link %#v", links, retainedLink)
	}

	var watchlists, instruments, events, deletedLinks int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT
    (SELECT count(*) FROM watchlists WHERE id = $1),
    (SELECT count(*) FROM watchlist_instruments WHERE watchlist_id = $1),
    (SELECT count(*) FROM economic_events WHERE id IN ($2, $3)),
    (SELECT count(*) FROM watchlist_event_links WHERE id = $4)
`, storedWatchlist.ID, deletedEventID, retainedEventID, deletedLink.ID).Scan(
		&watchlists, &instruments, &events, &deletedLinks,
	); err != nil {
		t.Fatalf("query preserved references: %v", err)
	}
	if watchlists != 1 || instruments != 2 || events != 2 || deletedLinks != 0 {
		t.Errorf(
			"preserved row counts = (%d, %d, %d, %d), want (1, 2, 2, 0)",
			watchlists, instruments, events, deletedLinks,
		)
	}
}

func TestRepositoryDeleteEventLinkReturnsNotFound(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Missing references", Symbols: []string{"SPY"},
	}, "creator")
	linkedEventID := "00000000-0000-0000-0000-000000000811"
	unlinkedEventID := "00000000-0000-0000-0000-000000000812"
	missingID := "00000000-0000-0000-0000-000000000899"
	insertEconomicEvent(t, database.Pool, linkedEventID, "linked", time.Now())
	insertEconomicEvent(t, database.Pool, unlinkedEventID, "unlinked", time.Now().Add(time.Hour))
	if _, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, "SPY", linkedEventID, "analyst",
	); err != nil {
		t.Fatalf("CreateEventLink() error = %v", err)
	}

	tests := []struct {
		name        string
		watchlistID string
		symbol      string
		eventID     string
	}{
		{name: "watchlist", watchlistID: missingID, symbol: "SPY", eventID: linkedEventID},
		{name: "instrument", watchlistID: storedWatchlist.ID, symbol: "EURUSD", eventID: linkedEventID},
		{name: "event", watchlistID: storedWatchlist.ID, symbol: "SPY", eventID: missingID},
		{name: "association", watchlistID: storedWatchlist.ID, symbol: "SPY", eventID: unlinkedEventID},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := repository.DeleteEventLink(t.Context(), test.watchlistID, test.symbol, test.eventID)
			if !errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "delete watchlist event link") {
				t.Fatalf("DeleteEventLink() error = %v, want contextual pgx.ErrNoRows", err)
			}
		})
	}
	assertEventLinkCount(t, database.Pool, storedWatchlist.ID, 1)
}

func TestRepositoryDeleteEventLinkValidatesBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	validID := "00000000-0000-0000-0000-000000000001"
	tests := []struct {
		name        string
		watchlistID string
		symbol      string
		eventID     string
	}{
		{name: "watchlist ID", watchlistID: "bad", symbol: "SPY", eventID: validID},
		{name: "symbol", watchlistID: validID, symbol: " ", eventID: validID},
		{name: "event ID", watchlistID: validID, symbol: "SPY", eventID: "bad"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := repository.DeleteEventLink(
				t.Context(), test.watchlistID, test.symbol, test.eventID,
			); err == nil {
				t.Fatal("DeleteEventLink() error = nil, want validation error")
			}
		})
	}
}

func TestRepositoryDeleteEventLinkPreservesCancellation(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	validID := "00000000-0000-0000-0000-000000000001"

	if err := repository.DeleteEventLink(ctx, validID, "SPY", validID); !errors.Is(err, context.Canceled) {
		t.Errorf("DeleteEventLink() error = %v, want context.Canceled", err)
	}
}

func TestRepositoryRollsBackEventLinkDeletion(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Protected link", Symbols: []string{"SPY"},
	}, "creator")
	eventID := "00000000-0000-0000-0000-000000000821"
	insertEconomicEvent(t, database.Pool, eventID, "protected-link", time.Now())
	created, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, "SPY", eventID, "analyst",
	)
	if err != nil {
		t.Fatalf("CreateEventLink() error = %v", err)
	}

	if _, err := database.Pool.Exec(t.Context(), `
CREATE TABLE watchlist_event_link_delete_references (
    link_id uuid PRIMARY KEY REFERENCES watchlist_event_links (id)
)
`); err != nil {
		t.Fatalf("create restricting reference: %v", err)
	}
	if _, err := database.Pool.Exec(
		t.Context(),
		`INSERT INTO watchlist_event_link_delete_references (link_id) VALUES ($1)`,
		created.ID,
	); err != nil {
		t.Fatalf("insert restricting reference: %v", err)
	}

	err = repository.DeleteEventLink(t.Context(), storedWatchlist.ID, "SPY", eventID)
	if err == nil || !strings.Contains(err.Error(), "delete watchlist event link") {
		t.Fatalf("DeleteEventLink() error = %v, want contextual deletion failure", err)
	}

	links, err := repository.EventLinks(t.Context(), storedWatchlist.ID, "SPY", 10)
	if err != nil {
		t.Fatalf("EventLinks() after rollback error = %v", err)
	}
	if !reflect.DeepEqual(links, []watchlist.StoredEventLink{created}) {
		t.Errorf("EventLinks() after rollback = %#v, want %#v", links, []watchlist.StoredEventLink{created})
	}
	var watchlists, instruments, events int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT
    (SELECT count(*) FROM watchlists WHERE id = $1),
    (SELECT count(*) FROM watchlist_instruments WHERE watchlist_id = $1),
    (SELECT count(*) FROM economic_events WHERE id = $2)
`, storedWatchlist.ID, eventID).Scan(&watchlists, &instruments, &events); err != nil {
		t.Fatalf("query references after rollback: %v", err)
	}
	if watchlists != 1 || instruments != 1 || events != 1 {
		t.Errorf("reference counts after rollback = (%d, %d, %d), want (1, 1, 1)", watchlists, instruments, events)
	}
}
