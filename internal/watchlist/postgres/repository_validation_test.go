package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

func TestWatchlistSchemaConstraints(t *testing.T) {
	database := openDatabase(t)
	ctx := t.Context()

	if _, err := database.Pool.Exec(ctx, `
INSERT INTO watchlists (name, created_by, updated_by) VALUES (' ', 'user', 'user')
`); err == nil {
		t.Fatal("blank watchlist name insert succeeded")
	}

	var watchlistID string
	if err := database.Pool.QueryRow(ctx, `
INSERT INTO watchlists (name, created_by, updated_by)
VALUES ('Schema checks', 'user', 'user')
RETURNING id::text
`).Scan(&watchlistID); err != nil {
		t.Fatalf("insert valid watchlist: %v", err)
	}
	insertInstrument := func(position int, symbol string) error {
		_, err := database.Pool.Exec(ctx, `
INSERT INTO watchlist_instruments (watchlist_id, position, symbol, created_by, updated_by)
VALUES ($1, $2, $3, 'user', 'user')
`, watchlistID, position, symbol)
		return err
	}
	if err := insertInstrument(0, " EURUSD "); err == nil {
		t.Error("untrimmed symbol insert succeeded")
	}
	if err := insertInstrument(-1, "NEGATIVE"); err == nil {
		t.Error("negative position insert succeeded")
	}
	if err := insertInstrument(0, "EURUSD"); err != nil {
		t.Fatalf("valid instrument insert: %v", err)
	}
	if err := insertInstrument(0, "SPY"); err == nil {
		t.Error("duplicate position insert succeeded")
	}
	if err := insertInstrument(1, "EURUSD"); err == nil {
		t.Error("duplicate symbol insert succeeded")
	}
}

func TestRepositoryValidatesBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	valid := watchlist.Definition{Name: "Macro", Symbols: []string{"EURUSD"}}
	definitions := []struct {
		name       string
		definition watchlist.Definition
		actor      string
	}{
		{name: "blank name", definition: watchlist.Definition{Name: " ", Symbols: []string{"EURUSD"}}, actor: "user"},
		{name: "missing symbols", definition: watchlist.Definition{Name: "Macro"}, actor: "user"},
		{name: "blank symbol", definition: watchlist.Definition{Name: "Macro", Symbols: []string{" "}}, actor: "user"},
		{name: "normalized duplicate", definition: watchlist.Definition{Name: "Macro", Symbols: []string{"spy", " SPY "}}, actor: "user"},
		{name: "blank actor", definition: valid, actor: " "},
	}
	for _, test := range definitions {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.CreateWatchlist(t.Context(), test.definition, test.actor); err == nil {
				t.Fatal("CreateWatchlist() error = nil, want validation error")
			}
		})
	}

	for _, id := range []string{"", "not-a-uuid"} {
		if _, err := repository.Watchlist(t.Context(), id); err == nil {
			t.Fatalf("Watchlist(%q) error = nil, want validation error", id)
		}
		if _, err := repository.UpdateWatchlist(t.Context(), id, valid, "user"); err == nil {
			t.Fatalf("UpdateWatchlist(%q) error = nil, want validation error", id)
		}
		if err := repository.DeleteWatchlist(t.Context(), id); err == nil {
			t.Fatalf("DeleteWatchlist(%q) error = nil, want validation error", id)
		}
	}
	for _, test := range definitions {
		t.Run("update "+test.name, func(t *testing.T) {
			if _, err := repository.UpdateWatchlist(
				t.Context(), "00000000-0000-0000-0000-000000000001", test.definition, test.actor,
			); err == nil {
				t.Fatal("UpdateWatchlist() error = nil, want validation error")
			}
		})
	}
	for _, limit := range []int{0, watchlist.MaxWatchlistsLimit + 1} {
		if _, err := repository.Watchlists(t.Context(), limit); err == nil {
			t.Fatalf("Watchlists(%d) error = nil, want validation error", limit)
		}
	}
}

func TestRepositoryPreservesCancellation(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := repository.CreateWatchlist(ctx, watchlist.Definition{Name: "Macro", Symbols: []string{"SPY"}}, "user"); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateWatchlist() error = %v, want context.Canceled", err)
	}
	if _, err := repository.UpdateWatchlist(
		ctx,
		"00000000-0000-0000-0000-000000000001",
		watchlist.Definition{Name: "Macro", Symbols: []string{"SPY"}},
		"user",
	); !errors.Is(err, context.Canceled) {
		t.Errorf("UpdateWatchlist() error = %v, want context.Canceled", err)
	}
	if _, err := repository.Watchlists(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Watchlists() error = %v, want context.Canceled", err)
	}
	if err := repository.DeleteWatchlist(
		ctx, "00000000-0000-0000-0000-000000000001",
	); !errors.Is(err, context.Canceled) {
		t.Errorf("DeleteWatchlist() error = %v, want context.Canceled", err)
	}
}

func TestNewRepositoryRequiresPostgreSQL(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository() error = nil, want missing database error")
	}
}
