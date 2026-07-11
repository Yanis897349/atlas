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
SET created_at = CASE WHEN id = $1 THEN $2 ELSE $3 END,
    updated_at = CASE WHEN id = $1 THEN $2 ELSE $3 END
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
	if _, err := repository.Watchlists(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Watchlists() error = %v, want context.Canceled", err)
	}
}

func TestNewRepositoryRequiresPostgreSQL(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository() error = nil, want missing database error")
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
