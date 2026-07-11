package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRunLinksAndListsWatchlistEventsEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	if err := Run(t.Context(), []string{
		"create-watchlist", "--name", "Macro events", "--actor", "creator",
		"--symbol", " spy ", "--symbol", "EURUSD",
	}, dependencies); err != nil {
		t.Fatalf("Run(create-watchlist) error = %v", err)
	}
	var createdWatchlist watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &createdWatchlist); err != nil {
		t.Fatalf("decode watchlist: %v", err)
	}

	tieTime := time.Date(2026, time.July, 18, 14, 0, 0, 0, time.UTC)
	events := []struct {
		id          string
		externalID  string
		scheduledAt time.Time
	}{
		{id: "00000000-0000-0000-0000-000000000103", externalID: "earliest", scheduledAt: tieTime.Add(-time.Hour)},
		{id: "00000000-0000-0000-0000-000000000102", externalID: "tie-later-id", scheduledAt: tieTime},
		{id: "00000000-0000-0000-0000-000000000101", externalID: "tie-earlier-id", scheduledAt: tieTime},
	}
	for _, event := range events {
		insertCommandEconomicEvent(t, database, event.id, event.externalID, event.scheduledAt)
		stdout.Reset()
		if err := Run(t.Context(), []string{
			"link-watchlist-event", "--id", createdWatchlist.ID, "--symbol", " sPy ",
			"--event-id", event.id, "--actor", " analyst ",
		}, dependencies); err != nil {
			t.Fatalf("Run(link-watchlist-event %q) error = %v", event.externalID, err)
		}
		var output watchlistEventOutput
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			t.Fatalf("decode linked event: %v", err)
		}
		if output.WatchlistID != createdWatchlist.ID || output.Symbol != "SPY" ||
			output.Event.ID != event.id || output.Event.ExternalEventID != event.externalID ||
			output.Event.SourceURL != "https://example.com/events/"+event.externalID ||
			output.CreatedBy != "analyst" || output.UpdatedBy != "analyst" ||
			output.Event.CreatedBy != "calendar-user" || output.Event.UpdatedBy != "calendar-user" {
			t.Errorf("linked output = %#v, want complete canonical link", output)
		}
		assertWatchlistEventOutputUTC(t, output)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{
		"watchlist-events", "--id", createdWatchlist.ID, "--symbol", "SPy", "--limit", "2",
	}, dependencies); err != nil {
		t.Fatalf("Run(watchlist-events) error = %v", err)
	}
	var output []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode watchlist events: %v", err)
	}
	tieIDs := []string{events[1].id, events[2].id}
	sort.Strings(tieIDs)
	wantIDs := []string{events[0].id, tieIDs[0]}
	if len(output) != 2 || output[0].Event.ID != wantIDs[0] || output[1].Event.ID != wantIDs[1] {
		t.Errorf("listed event IDs = %#v, want %v", output, wantIDs)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{
		"watchlist-events", "--id", createdWatchlist.ID, "--symbol", "EURUSD", "--limit", "10",
	}, dependencies); err != nil {
		t.Fatalf("Run(watchlist-events empty) error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("empty stdout = %q, want []", stdout.String())
	}

	stdout.Reset()
	err := Run(t.Context(), []string{
		"link-watchlist-event", "--id", createdWatchlist.ID, "--symbol", "SPY",
		"--event-id", events[0].id, "--actor", "second",
	}, dependencies)
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23505" {
		t.Fatalf("duplicate link error = %v, want unique violation", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("duplicate link stdout = %q, want no output", stdout.String())
	}

	missingID := "00000000-0000-0000-0000-000000000999"
	for _, test := range []struct {
		name      string
		arguments []string
	}{
		{name: "link watchlist", arguments: []string{"link-watchlist-event", "--id", missingID, "--symbol", "SPY", "--event-id", events[0].id, "--actor", "analyst"}},
		{name: "link instrument", arguments: []string{"link-watchlist-event", "--id", createdWatchlist.ID, "--symbol", "DXY", "--event-id", events[0].id, "--actor", "analyst"}},
		{name: "link event", arguments: []string{"link-watchlist-event", "--id", createdWatchlist.ID, "--symbol", "SPY", "--event-id", missingID, "--actor", "analyst"}},
		{name: "list watchlist", arguments: []string{"watchlist-events", "--id", missingID, "--symbol", "SPY", "--limit", "10"}},
		{name: "list instrument", arguments: []string{"watchlist-events", "--id", createdWatchlist.ID, "--symbol", "DXY", "--limit", "10"}},
	} {
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

func insertCommandEconomicEvent(
	t *testing.T,
	database postgrestest.Database,
	id, externalID string,
	scheduledAt time.Time,
) {
	t.Helper()
	_, err := database.Pool.Exec(t.Context(), `
INSERT INTO economic_events (
    id, source, external_event_id, name, region, event_type, scheduled_at,
    source_url, retrieved_at, created_by, updated_by
)
VALUES ($1, 'test-calendar', $2, 'CPI release', 'united_states', 'inflation', $3,
        $4, $5, 'calendar-user', 'calendar-user')
`, id, externalID, scheduledAt, "https://example.com/events/"+externalID, scheduledAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("insert economic event: %v", err)
	}
}

func assertWatchlistEventOutputUTC(t *testing.T, output watchlistEventOutput) {
	t.Helper()
	for _, value := range []string{
		output.CreatedAt, output.UpdatedAt, output.Event.ScheduledAt,
		output.Event.RetrievedAt, output.Event.CreatedAt, output.Event.UpdatedAt,
	} {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil || parsed.Location() != time.UTC {
			t.Errorf("timestamp %q = (%v, %v), want UTC RFC3339Nano", value, parsed, err)
		}
	}
}
