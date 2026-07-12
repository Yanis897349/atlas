package postgres

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestRepositoryCreatesAndRetrievesWatchlist(t *testing.T) {
	database := openDatabase(t)
	repository, err := NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	stored, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name:    "  Macro focus  ",
		Symbols: []string{" eurusd ", "SpY", "brk.b"},
	}, "  watchlist-user  ")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}
	if !validUUID(stored.ID) {
		t.Errorf("stored ID = %q, want UUID", stored.ID)
	}
	if stored.Name != "Macro focus" || !reflect.DeepEqual(stored.Symbols, []string{"EURUSD", "SPY", "BRK.B"}) {
		t.Errorf("stored definition = %#v", stored.Definition)
	}
	if stored.CreatedBy != "watchlist-user" || stored.UpdatedBy != "watchlist-user" ||
		stored.CreatedAt.IsZero() || !stored.CreatedAt.Equal(stored.UpdatedAt) {
		t.Errorf("stored audit = %#v", stored)
	}
	if stored.CreatedAt.Location() != time.UTC || stored.UpdatedAt.Location() != time.UTC {
		t.Errorf("stored audit locations = (%v, %v), want UTC", stored.CreatedAt.Location(), stored.UpdatedAt.Location())
	}

	got, err := repository.Watchlist(t.Context(), stored.ID)
	if err != nil {
		t.Fatalf("Watchlist() error = %v", err)
	}
	if !reflect.DeepEqual(got, stored) {
		t.Errorf("Watchlist() = %#v, want %#v", got, stored)
	}

	var auditedInstruments int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*)
FROM watchlist_instruments
WHERE created_by = 'watchlist-user' AND updated_by = 'watchlist-user'
`).Scan(&auditedInstruments); err != nil {
		t.Fatalf("query instrument audit: %v", err)
	}
	if auditedInstruments != 3 {
		t.Errorf("audited instrument count = %d, want 3", auditedInstruments)
	}
}

func TestRepositoryUpdatesAndRetrievesWatchlist(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	created, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Original", Symbols: []string{"SPY", "EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}

	createdAt := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	if _, err := database.Pool.Exec(t.Context(), `
UPDATE watchlists SET created_at = $2, updated_at = $3 WHERE id = $1
`, created.ID, createdAt, updatedAt); err != nil {
		t.Fatalf("set deterministic audit times: %v", err)
	}
	before, err := repository.Watchlist(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Watchlist() before update error = %v", err)
	}

	updated, err := repository.UpdateWatchlist(t.Context(), created.ID, watchlist.Definition{
		Name: "  Updated macro  ", Symbols: []string{" brk.b ", "qqq", " EurUsd "},
	}, "  editor  ")
	if err != nil {
		t.Fatalf("UpdateWatchlist() error = %v", err)
	}
	if updated.ID != before.ID || updated.Name != "Updated macro" ||
		!reflect.DeepEqual(updated.Symbols, []string{"BRK.B", "QQQ", "EURUSD"}) {
		t.Errorf("UpdateWatchlist() = %#v, want complete normalized replacement", updated)
	}
	if !updated.CreatedAt.Equal(before.CreatedAt) || updated.CreatedBy != before.CreatedBy {
		t.Errorf("creation audit = (%v, %q), want (%v, %q)",
			updated.CreatedAt, updated.CreatedBy, before.CreatedAt, before.CreatedBy)
	}
	if !updated.UpdatedAt.After(before.UpdatedAt) || updated.UpdatedBy != "editor" {
		t.Errorf("update audit = (%v, %q), want time after %v and editor", updated.UpdatedAt, updated.UpdatedBy, before.UpdatedAt)
	}
	if updated.CreatedAt.Location() != time.UTC || updated.UpdatedAt.Location() != time.UTC {
		t.Errorf("updated audit locations = (%v, %v), want UTC", updated.CreatedAt.Location(), updated.UpdatedAt.Location())
	}

	got, err := repository.Watchlist(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Watchlist() after update error = %v", err)
	}
	if !reflect.DeepEqual(got, updated) {
		t.Errorf("Watchlist() = %#v, want %#v", got, updated)
	}

	var instruments, auditedInstruments int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*), count(*) FILTER (WHERE created_by = 'editor' AND updated_by = 'editor')
FROM watchlist_instruments
WHERE watchlist_id = $1
`, created.ID).Scan(&instruments, &auditedInstruments); err != nil {
		t.Fatalf("query replacement instrument audit: %v", err)
	}
	if instruments != 3 || auditedInstruments != 3 {
		t.Errorf("replacement instrument counts = (%d, %d), want (3, 3)", instruments, auditedInstruments)
	}
}

func TestRepositoryPersistsUnicodeSymbolsWithGoCanonicalization(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	input := []string{"ß", "é", "ﬀ"}
	want := make([]string, len(input))
	for index, symbol := range input {
		want[index] = strings.ToUpper(symbol)
	}

	stored, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Unicode", Symbols: input,
	}, "watchlist-user")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}
	if !reflect.DeepEqual(stored.Symbols, want) {
		t.Fatalf("CreateWatchlist() symbols = %v, want %v", stored.Symbols, want)
	}

	got, err := repository.Watchlist(t.Context(), stored.ID)
	if err != nil {
		t.Fatalf("Watchlist() error = %v", err)
	}
	if !reflect.DeepEqual(got.Symbols, want) {
		t.Errorf("Watchlist() symbols = %v, want %v", got.Symbols, want)
	}
}

func TestRepositoryWatchlistsOrdersAndLimits(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)

	stored := make([]watchlist.StoredWatchlist, 0, 3)
	for _, name := range []string{"First", "Second", "Newest"} {
		created, err := repository.CreateWatchlist(
			t.Context(),
			watchlist.Definition{Name: name, Symbols: []string{name}},
			"watchlist-user",
		)
		if err != nil {
			t.Fatalf("CreateWatchlist(%q) error = %v", name, err)
		}
		stored = append(stored, created)
	}

	tieTime := time.Date(2026, time.July, 12, 10, 0, 0, 0, time.UTC)
	newestTime := tieTime.Add(time.Hour)
	if _, err := database.Pool.Exec(t.Context(), `
UPDATE watchlists
SET created_at = CASE WHEN id = $1 THEN $2::timestamptz ELSE $3::timestamptz END,
    updated_at = CASE WHEN id = $1 THEN $2::timestamptz ELSE $3::timestamptz END
`, stored[2].ID, newestTime, tieTime); err != nil {
		t.Fatalf("set deterministic creation times: %v", err)
	}

	tiedIDs := []string{stored[0].ID, stored[1].ID}
	sort.Strings(tiedIDs)
	got, err := repository.Watchlists(t.Context(), 2)
	if err != nil {
		t.Fatalf("Watchlists() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != stored[2].ID || got[1].ID != tiedIDs[0] {
		t.Fatalf("Watchlists() IDs = %v, want [%s %s]", watchlistIDs(got), stored[2].ID, tiedIDs[0])
	}
}

func TestRepositoryReturnsEmptyWatchlistListAndMissingID(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)

	got, err := repository.Watchlists(t.Context(), 10)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Watchlists() = (%#v, %v), want non-nil empty list", got, err)
	}
	_, err = repository.Watchlist(t.Context(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Watchlist() error = %v, want pgx.ErrNoRows", err)
	}
	_, err = repository.UpdateWatchlist(
		t.Context(),
		"00000000-0000-0000-0000-000000000001",
		watchlist.Definition{Name: "Missing", Symbols: []string{"SPY"}},
		"watchlist-user",
	)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("UpdateWatchlist() error = %v, want pgx.ErrNoRows", err)
	}
	if err := repository.DeleteWatchlist(
		t.Context(), "00000000-0000-0000-0000-000000000001",
	); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("DeleteWatchlist() error = %v, want pgx.ErrNoRows", err)
	}
}

func TestRepositoryDeletesWatchlistAndCascadesInstruments(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	created, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Delete", Symbols: []string{"SPY", "EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}

	if err := repository.DeleteWatchlist(t.Context(), created.ID); err != nil {
		t.Fatalf("DeleteWatchlist() error = %v", err)
	}
	if _, err := repository.Watchlist(t.Context(), created.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Watchlist() error = %v, want pgx.ErrNoRows", err)
	}

	var instruments int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*) FROM watchlist_instruments WHERE watchlist_id = $1
`, created.ID).Scan(&instruments); err != nil {
		t.Fatalf("count watchlist instruments: %v", err)
	}
	if instruments != 0 {
		t.Errorf("instrument count = %d, want cascade deletion", instruments)
	}
}

func TestRepositoryRollsBackAtomicCreation(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	if _, err := database.Pool.Exec(t.Context(), `
ALTER TABLE watchlist_instruments
ADD CONSTRAINT chk_watchlist_instruments_test_rejection CHECK (symbol <> 'REJECT')
`); err != nil {
		t.Fatalf("add rejection constraint: %v", err)
	}

	_, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Rollback", Symbols: []string{"OK", "REJECT"},
	}, "watchlist-user")
	if err == nil || !strings.Contains(err.Error(), "insert watchlist instrument 1") {
		t.Fatalf("CreateWatchlist() error = %v, want contextual instrument failure", err)
	}

	var watchlists, instruments int
	if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM watchlists`).Scan(&watchlists); err != nil {
		t.Fatalf("count watchlists: %v", err)
	}
	if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM watchlist_instruments`).Scan(&instruments); err != nil {
		t.Fatalf("count instruments: %v", err)
	}
	if watchlists != 0 || instruments != 0 {
		t.Errorf("row counts = (%d, %d), want atomic rollback to zero", watchlists, instruments)
	}
}

func TestRepositoryRollsBackAtomicUpdate(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	created, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Original", Symbols: []string{"SPY", "EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}
	if _, err := database.Pool.Exec(t.Context(), `
ALTER TABLE watchlist_instruments
ADD CONSTRAINT chk_watchlist_instruments_test_update_rejection CHECK (symbol <> 'REJECT')
`); err != nil {
		t.Fatalf("add rejection constraint: %v", err)
	}

	_, err = repository.UpdateWatchlist(t.Context(), created.ID, watchlist.Definition{
		Name: "Changed", Symbols: []string{"OK", "REJECT"},
	}, "editor")
	if err == nil || !strings.Contains(err.Error(), "insert watchlist instrument 1") {
		t.Fatalf("UpdateWatchlist() error = %v, want contextual instrument failure", err)
	}

	got, err := repository.Watchlist(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Watchlist() after rollback error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("Watchlist() after rollback = %#v, want %#v", got, created)
	}
}

func TestRepositoryRollsBackAtomicDeletion(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	created, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Protected", Symbols: []string{"SPY", "EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}
	if _, err := database.Pool.Exec(t.Context(), `
CREATE TABLE watchlist_delete_references (
    watchlist_id uuid PRIMARY KEY REFERENCES watchlists (id)
)
`); err != nil {
		t.Fatalf("create restricting reference: %v", err)
	}
	if _, err := database.Pool.Exec(
		t.Context(), `INSERT INTO watchlist_delete_references (watchlist_id) VALUES ($1)`, created.ID,
	); err != nil {
		t.Fatalf("insert restricting reference: %v", err)
	}

	err = repository.DeleteWatchlist(t.Context(), created.ID)
	if err == nil || !strings.Contains(err.Error(), "delete watchlist") {
		t.Fatalf("DeleteWatchlist() error = %v, want contextual deletion failure", err)
	}

	got, err := repository.Watchlist(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Watchlist() after rollback error = %v", err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Errorf("Watchlist() after rollback = %#v, want %#v", got, created)
	}
}

func openDatabase(t *testing.T) postgrestest.Database {
	t.Helper()
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return database
}

func validUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil && id.Valid
}

func watchlistIDs(watchlists []watchlist.StoredWatchlist) []string {
	ids := make([]string, len(watchlists))
	for index, stored := range watchlists {
		ids[index] = stored.ID
	}
	return ids
}

type panicDB struct{}

func (panicDB) Begin(context.Context) (pgx.Tx, error) {
	panic("validation must happen before beginning a transaction")
}

func (panicDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("validation must happen before querying PostgreSQL")
}

func (panicDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("validation must happen before querying PostgreSQL")
}
