package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/jackc/pgx/v5"
)

func TestRunCreatesAndListsWatchlistsEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	create := func(name, actor string, symbols ...string) watchlistOutput {
		t.Helper()
		arguments := []string{"create-watchlist", "--name", name, "--actor", actor}
		for _, symbol := range symbols {
			arguments = append(arguments, "--symbol", symbol)
		}
		stdout := &bytes.Buffer{}
		dependencies.Stdout = stdout
		if err := Run(t.Context(), arguments, dependencies); err != nil {
			t.Fatalf("Run(create-watchlist %q) error = %v", name, err)
		}
		var output watchlistOutput
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			t.Fatalf("decode created watchlist: %v", err)
		}
		return output
	}

	first := create(" First ", " first-user ", " eurusd ", "SpY")
	second := create("Second", "second-user", "brk.b")
	if first.Name != "First" || first.CreatedBy != "first-user" || first.UpdatedBy != "first-user" ||
		!reflect.DeepEqual(first.Symbols, []string{"EURUSD", "SPY"}) {
		t.Fatalf("first created output = %#v, want normalized definition and audit", first)
	}

	var databaseNow time.Time
	if err := database.Pool.QueryRow(t.Context(), `SELECT statement_timestamp()`).Scan(&databaseNow); err != nil {
		t.Fatalf("query database time: %v", err)
	}
	firstTime := databaseNow.UTC().Add(-2 * time.Hour).Truncate(time.Second)
	secondTime := firstTime.Add(time.Hour)
	if _, err := database.Pool.Exec(t.Context(), `
UPDATE watchlists
SET created_at = CASE WHEN id = $1 THEN $2::timestamptz ELSE $3::timestamptz END,
    updated_at = CASE WHEN id = $1 THEN $2::timestamptz ELSE $3::timestamptz END
`, first.ID, firstTime, secondTime); err != nil {
		t.Fatalf("set deterministic watchlist times: %v", err)
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	if err := Run(t.Context(), []string{
		"update-watchlist",
		"--id", first.ID,
		"--name", " Updated first ",
		"--actor", " editor ",
		"--symbol", " dxy ",
		"--symbol", "Brk.b",
	}, dependencies); err != nil {
		t.Fatalf("Run(update-watchlist) error = %v", err)
	}
	var updated watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated watchlist: %v", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updated.UpdatedAt)
	if err != nil {
		t.Fatalf("parse updated timestamp: %v", err)
	}
	if updated.ID != first.ID || updated.Name != "Updated first" ||
		!reflect.DeepEqual(updated.Symbols, []string{"DXY", "BRK.B"}) || updated.CreatedBy != "first-user" ||
		updated.UpdatedBy != "editor" || updated.CreatedAt != formatWatchlistOutputTime(firstTime) ||
		!updatedAt.After(firstTime) {
		t.Errorf("updated output = %#v, want replaced definition with preserved creation metadata", updated)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{"watchlists", "--limit", "1"}, dependencies); err != nil {
		t.Fatalf("Run(watchlists) error = %v", err)
	}
	var output []watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode listed watchlists: %v", err)
	}
	if len(output) != 1 || output[0].ID != second.ID || output[0].Name != "Second" ||
		!reflect.DeepEqual(output[0].Symbols, []string{"BRK.B"}) || output[0].CreatedBy != "second-user" ||
		output[0].CreatedAt != formatWatchlistOutputTime(secondTime) {
		t.Errorf("listed output = %#v, want newest complete watchlist", output)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{"watchlist", "--id", first.ID}, dependencies); err != nil {
		t.Fatalf("Run(watchlist) error = %v", err)
	}
	var lookup watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &lookup); err != nil {
		t.Fatalf("decode looked-up watchlist: %v", err)
	}
	if !reflect.DeepEqual(lookup, updated) {
		t.Errorf("looked-up output = %#v, want updated watchlist %#v", lookup, updated)
	}

	stdout.Reset()
	err = Run(t.Context(), []string{"watchlist", "--id", "00000000-0000-0000-0000-000000000000"}, dependencies)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Run(watchlist missing) error = %v, want pgx.ErrNoRows", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("missing watchlist stdout = %q, want no output", stdout.String())
	}

	stdout.Reset()
	err = Run(t.Context(), []string{
		"update-watchlist",
		"--id", "00000000-0000-0000-0000-000000000000",
		"--name", "Missing",
		"--actor", "editor",
		"--symbol", "SPY",
	}, dependencies)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Run(update-watchlist missing) error = %v, want pgx.ErrNoRows", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("missing update stdout = %q, want no output", stdout.String())
	}

	var auditedInstruments int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*)
FROM watchlist_instruments
WHERE watchlist_id = $1 AND created_by = 'editor' AND updated_by = 'editor'
`, first.ID).Scan(&auditedInstruments); err != nil {
		t.Fatalf("query instrument audit: %v", err)
	}
	if auditedInstruments != 2 {
		t.Errorf("audited replacement instrument count = %d, want 2", auditedInstruments)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{"delete-watchlist", "--id", first.ID}, dependencies); err != nil {
		t.Fatalf("Run(delete-watchlist) error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("deleted watchlist stdout = %q, want no output", stdout.String())
	}
	if err := Run(t.Context(), []string{"watchlist", "--id", first.ID}, dependencies); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Run(watchlist after delete) error = %v, want pgx.ErrNoRows", err)
	}
	var remainingInstruments int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*) FROM watchlist_instruments WHERE watchlist_id = $1
`, first.ID).Scan(&remainingInstruments); err != nil {
		t.Fatalf("count instruments after command deletion: %v", err)
	}
	if remainingInstruments != 0 {
		t.Errorf("instrument count after command deletion = %d, want 0", remainingInstruments)
	}

	stdout.Reset()
	err = Run(t.Context(), []string{
		"delete-watchlist", "--id", "00000000-0000-0000-0000-000000000000",
	}, dependencies)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Run(delete-watchlist missing) error = %v, want pgx.ErrNoRows", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("missing deletion stdout = %q, want no output", stdout.String())
	}
}
