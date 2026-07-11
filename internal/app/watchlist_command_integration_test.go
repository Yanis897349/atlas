package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
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

	firstTime := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(time.Hour)
	if _, err := database.Pool.Exec(t.Context(), `
UPDATE watchlists
SET created_at = CASE WHEN id = $1 THEN $2 ELSE $3 END,
    updated_at = CASE WHEN id = $1 THEN $2 ELSE $3 END
`, first.ID, firstTime, secondTime); err != nil {
		t.Fatalf("set deterministic watchlist times: %v", err)
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	if err := Run(t.Context(), []string{"watchlists", "--limit", "1"}, dependencies); err != nil {
		t.Fatalf("Run(watchlists) error = %v", err)
	}
	var output []watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode listed watchlists: %v", err)
	}
	if len(output) != 1 || output[0].ID != second.ID || output[0].Name != "Second" ||
		!reflect.DeepEqual(output[0].Symbols, []string{"BRK.B"}) || output[0].CreatedBy != "second-user" ||
		output[0].CreatedAt != "2026-07-12T09:00:00Z" {
		t.Errorf("listed output = %#v, want newest complete watchlist", output)
	}

	var auditedInstruments int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT count(*)
FROM watchlist_instruments
WHERE watchlist_id = $1 AND created_by = 'first-user' AND updated_by = 'first-user'
`, first.ID).Scan(&auditedInstruments); err != nil {
		t.Fatalf("query instrument audit: %v", err)
	}
	if auditedInstruments != 2 {
		t.Errorf("audited instrument count = %d, want 2", auditedInstruments)
	}
}
