package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/jackc/pgx/v5"
)

func TestRunLinksWatchlistEventCandidatesEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	watchlistID := createCommandWatchlist(
		t,
		dependencies,
		stdout,
		"Candidate links",
		[]string{"DXY", "EURUSD", "SPY"},
	)

	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(4 * time.Hour)
	events := []struct {
		id          string
		externalID  string
		region      calendar.Region
		eventType   calendar.EventType
		scheduledAt time.Time
	}{
		{id: "00000000-0000-0000-0000-000000000200", externalID: "before", region: calendar.RegionUnitedStates, eventType: calendar.EventTypeInflation, scheduledAt: windowStart.Add(-time.Microsecond)},
		{id: "00000000-0000-0000-0000-000000000201", externalID: "start-us", region: calendar.RegionUnitedStates, eventType: calendar.EventTypeEmployment, scheduledAt: windowStart},
		{id: "00000000-0000-0000-0000-000000000202", externalID: "middle-eu", region: calendar.RegionEurozone, eventType: calendar.EventTypeInterestRateDecision, scheduledAt: windowStart.Add(2 * time.Hour)},
		{id: "00000000-0000-0000-0000-000000000203", externalID: "end-us", region: calendar.RegionUnitedStates, eventType: calendar.EventTypeGDP, scheduledAt: windowEnd},
		{id: "00000000-0000-0000-0000-000000000204", externalID: "after", region: calendar.RegionUnitedStates, eventType: calendar.EventTypePMI, scheduledAt: windowEnd.Add(time.Microsecond)},
	}
	for _, event := range events {
		insertCandidateCommandEconomicEvent(
			t,
			database,
			event.id,
			event.externalID,
			event.region,
			event.eventType,
			event.scheduledAt,
		)
	}

	arguments := []string{
		"link-watchlist-events",
		"--id", watchlistID,
		"--from", "2026-08-01T10:00:00+02:00",
		"--to", "2026-08-01T12:00:00Z",
		"--limit", "2",
		"--actor", " classifier ",
	}
	stdout.Reset()
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(link-watchlist-events) error = %v", err)
	}
	var firstOutput []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &firstOutput); err != nil {
		t.Fatalf("decode candidate links: %v", err)
	}
	wantReferences := [][2]string{
		{"DXY", events[1].id},
		{"EURUSD", events[1].id},
		{"EURUSD", events[2].id},
		{"SPY", events[1].id},
	}
	assertCandidateCommandLinks(t, firstOutput, watchlistID, wantReferences, "classifier")
	for _, output := range firstOutput {
		if output.Event.ID == events[3].id {
			t.Errorf("globally limited output contains third candidate %q", events[3].id)
		}
	}

	stdout.Reset()
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(link-watchlist-events retry) error = %v", err)
	}
	var retriedOutput []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &retriedOutput); err != nil {
		t.Fatalf("decode retried candidate links: %v", err)
	}
	if !reflect.DeepEqual(retriedOutput, firstOutput) {
		t.Errorf("retried output = %#v, want idempotent %#v", retriedOutput, firstOutput)
	}

	stdout.Reset()
	if err := Run(t.Context(), []string{
		"link-watchlist-events",
		"--id", watchlistID,
		"--from", "2026-08-01T12:00:00Z",
		"--to", "2026-08-01T12:00:00Z",
		"--limit", "10",
		"--actor", "boundary-classifier",
	}, dependencies); err != nil {
		t.Fatalf("Run(link-watchlist-events inclusive end) error = %v", err)
	}
	var boundaryOutput []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &boundaryOutput); err != nil {
		t.Fatalf("decode inclusive end candidate links: %v", err)
	}
	assertCandidateCommandLinks(t, boundaryOutput, watchlistID, [][2]string{
		{"DXY", events[3].id},
		{"EURUSD", events[3].id},
		{"SPY", events[3].id},
	}, "boundary-classifier")

	emptyWatchlistID := createCommandWatchlist(
		t,
		dependencies,
		stdout,
		"No Eurozone exposure",
		[]string{"DXY"},
	)
	stdout.Reset()
	if err := Run(t.Context(), []string{
		"link-watchlist-events",
		"--id", emptyWatchlistID,
		"--from", "2026-08-01T10:00:00Z",
		"--to", "2026-08-01T10:00:00Z",
		"--limit", "10",
		"--actor", "classifier",
	}, dependencies); err != nil {
		t.Fatalf("Run(link-watchlist-events empty) error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("empty stdout = %q, want []", stdout.String())
	}

	stdout.Reset()
	err := Run(t.Context(), []string{
		"link-watchlist-events",
		"--id", "00000000-0000-0000-0000-000000000999",
		"--from", "2026-08-01T08:00:00Z",
		"--to", "2026-08-01T12:00:00Z",
		"--limit", "10",
		"--actor", "classifier",
	}, dependencies)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("Run(link-watchlist-events missing watchlist) error = %v, want pgx.ErrNoRows", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("missing watchlist stdout = %q, want no output", stdout.String())
	}
}

func createCommandWatchlist(
	t *testing.T,
	dependencies Dependencies,
	stdout *bytes.Buffer,
	name string,
	symbols []string,
) string {
	t.Helper()
	arguments := []string{"create-watchlist", "--name", name, "--actor", "creator"}
	for _, symbol := range symbols {
		arguments = append(arguments, "--symbol", symbol)
	}
	stdout.Reset()
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(create-watchlist) error = %v", err)
	}
	var output watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode created watchlist: %v", err)
	}
	return output.ID
}

func insertCandidateCommandEconomicEvent(
	t *testing.T,
	database postgrestest.Database,
	id string,
	externalID string,
	region calendar.Region,
	eventType calendar.EventType,
	scheduledAt time.Time,
) {
	t.Helper()
	_, err := database.Pool.Exec(t.Context(), `
INSERT INTO economic_events (
    id, source, external_event_id, name, region, event_type, scheduled_at,
    source_url, retrieved_at, created_by, updated_by
)
VALUES ($1, 'test-calendar', $2, $3, $4, $5, $6, $7, $8, 'calendar-user', 'calendar-user')
`, id, externalID, "Economic event "+externalID, region, eventType, scheduledAt,
		"https://example.com/events/"+externalID, scheduledAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("insert economic event: %v", err)
	}
}

func assertCandidateCommandLinks(
	t *testing.T,
	output []watchlistEventOutput,
	watchlistID string,
	wantReferences [][2]string,
	wantActor string,
) {
	t.Helper()
	if len(output) != len(wantReferences) {
		t.Fatalf("output count = %d, want %d: %#v", len(output), len(wantReferences), output)
	}
	for index, want := range wantReferences {
		link := output[index]
		if link.WatchlistID != watchlistID || link.Symbol != want[0] || link.Event.ID != want[1] {
			t.Errorf(
				"output[%d] reference = (%q, %q, %q), want (%q, %q, %q)",
				index,
				link.WatchlistID,
				link.Symbol,
				link.Event.ID,
				watchlistID,
				want[0],
				want[1],
			)
		}
		if link.ID == "" || link.CreatedBy != wantActor || link.UpdatedBy != wantActor ||
			link.Event.Source != "test-calendar" || link.Event.SourceURL == "" ||
			link.Event.CreatedBy != "calendar-user" || link.Event.UpdatedBy != "calendar-user" {
			t.Errorf("output[%d] = %#v, want complete canonical link and citation", index, link)
		}
		assertWatchlistEventOutputUTC(t, link)
	}
}
