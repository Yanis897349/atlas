package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/jackc/pgx/v5"
)

func TestRunUnlinksOneWatchlistEventEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	stdout := &bytes.Buffer{}
	dependencies := Dependencies{
		Getenv: applicationDatabaseEnv(database.URL),
		Stdout: stdout,
	}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{
		"create-watchlist", "--name", "Unlink events", "--actor", "creator",
		"--symbol", "SPY", "--symbol", "EURUSD",
	}, dependencies); err != nil {
		t.Fatalf("Run(create-watchlist) error = %v", err)
	}
	var createdWatchlist watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &createdWatchlist); err != nil {
		t.Fatalf("decode watchlist: %v", err)
	}

	eventIDs := []string{
		"00000000-0000-0000-0000-000000000201",
		"00000000-0000-0000-0000-000000000202",
	}
	for index, eventID := range eventIDs {
		insertCommandEconomicEvent(
			t, database, eventID, "unlink-event-"+eventID,
			time.Date(2026, time.July, 20+index, 12, 0, 0, 0, time.UTC),
		)
		stdout.Reset()
		if err := Run(t.Context(), []string{
			"link-watchlist-event", "--id", createdWatchlist.ID, "--symbol", "SPY",
			"--event-id", eventID, "--actor", "analyst",
		}, dependencies); err != nil {
			t.Fatalf("Run(link-watchlist-event) error = %v", err)
		}
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{
		"unlink-watchlist-event", "--id", createdWatchlist.ID, "--symbol", " sPy ",
		"--event-id", eventIDs[0],
	}, dependencies); err != nil {
		t.Fatalf("Run(unlink-watchlist-event) error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("unlink stdout = %q, want no output", stdout.String())
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{
		"watchlist-events", "--id", createdWatchlist.ID, "--symbol", "SPY", "--limit", "10",
	}, dependencies); err != nil {
		t.Fatalf("Run(watchlist-events) error = %v", err)
	}
	var retained []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &retained); err != nil {
		t.Fatalf("decode retained links: %v", err)
	}
	if len(retained) != 1 || retained[0].Event.ID != eventIDs[1] {
		t.Errorf("retained links = %#v, want only event %s", retained, eventIDs[1])
	}

	var watchlists, instruments, events int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT
    (SELECT count(*) FROM watchlists WHERE id = $1),
    (SELECT count(*) FROM watchlist_instruments WHERE watchlist_id = $1),
    (SELECT count(*) FROM economic_events WHERE id = ANY($2::uuid[]))
`, createdWatchlist.ID, eventIDs).Scan(&watchlists, &instruments, &events); err != nil {
		t.Fatalf("query preserved references: %v", err)
	}
	if watchlists != 1 || instruments != 2 || events != 2 {
		t.Errorf("preserved row counts = (%d, %d, %d), want (1, 2, 2)", watchlists, instruments, events)
	}

	missingID := "00000000-0000-0000-0000-000000000299"
	tests := []struct {
		name      string
		arguments []string
	}{
		{name: "watchlist", arguments: []string{"unlink-watchlist-event", "--id", missingID, "--symbol", "SPY", "--event-id", eventIDs[1]}},
		{name: "instrument", arguments: []string{"unlink-watchlist-event", "--id", createdWatchlist.ID, "--symbol", "DXY", "--event-id", eventIDs[1]}},
		{name: "event", arguments: []string{"unlink-watchlist-event", "--id", createdWatchlist.ID, "--symbol", "SPY", "--event-id", missingID}},
		{name: "association", arguments: []string{"unlink-watchlist-event", "--id", createdWatchlist.ID, "--symbol", "SPY", "--event-id", eventIDs[0]}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout.Reset()
			err := Run(t.Context(), test.arguments, dependencies)
			if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("Run() error = %v, want pgx.ErrNoRows", err)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want no output", stdout.String())
			}
		})
	}
}
