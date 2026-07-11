package app

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
)

func TestRunReadsStoredDailyBriefsEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	repository, err := newDailyBriefRepository(database.Pool)
	if err != nil {
		t.Fatalf("newDailyBriefRepository() error = %v", err)
	}
	sourceID, eventID := persistDailyBriefReferences(t, database)

	windowStart := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(4 * time.Hour)
	type storedAt struct {
		name      string
		region    calendar.Region
		createdAt time.Time
	}
	briefs := []storedAt{
		{name: "before", region: calendar.RegionUnitedStates, createdAt: windowStart.Add(-time.Microsecond)},
		{name: "start", region: calendar.RegionUnitedStates, createdAt: windowStart},
		{name: "tie-a", region: calendar.RegionUnitedStates, createdAt: windowEnd},
		{name: "tie-b", region: calendar.RegionUnitedStates, createdAt: windowEnd},
		{name: "after", region: calendar.RegionUnitedStates, createdAt: windowEnd.Add(time.Microsecond)},
		{name: "other-region", region: calendar.RegionEurozone, createdAt: windowStart.Add(time.Hour)},
	}
	storedByName := make(map[string]storedDailyBrief, len(briefs))
	for _, candidate := range briefs {
		brief := persistedDailyBriefFixture(sourceID, eventID)
		brief.region = candidate.region
		stored, persistErr := repository.PersistDailyBrief(t.Context(), brief, "brief-worker")
		if persistErr != nil {
			t.Fatalf("PersistDailyBrief(%q) error = %v", candidate.name, persistErr)
		}
		if _, updateErr := database.Pool.Exec(
			t.Context(),
			`UPDATE daily_briefs SET created_at = $1, updated_at = $1 WHERE id = $2`,
			candidate.createdAt,
			stored.ID,
		); updateErr != nil {
			t.Fatalf("set %q creation time: %v", candidate.name, updateErr)
		}
		stored.CreatedAt = candidate.createdAt
		stored.UpdatedAt = candidate.createdAt
		storedByName[candidate.name] = stored
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	err = Run(t.Context(), []string{
		"daily-briefs",
		"--region", "united_states",
		"--from", "2026-07-11T12:00:00+02:00",
		"--to", "2026-07-11T16:00:00+02:00",
		"--limit", "3",
	}, dependencies)
	if err != nil {
		t.Fatalf("Run(daily-briefs) error = %v", err)
	}

	var output []dailyBriefOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command JSON: %v", err)
	}
	tieIDs := []string{storedByName["tie-a"].ID, storedByName["tie-b"].ID}
	sort.Strings(tieIDs)
	wantIDs := []string{tieIDs[0], tieIDs[1], storedByName["start"].ID}
	if len(output) != len(wantIDs) {
		t.Fatalf("output count = %d, want %d", len(output), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		if output[index].ID != wantID {
			t.Errorf("output[%d].ID = %q, want %q", index, output[index].ID, wantID)
		}
		if output[index].Region != calendar.RegionUnitedStates || output[index].Provider != "openai" ||
			output[index].Model != "test-model" || output[index].CreatedBy != "brief-worker" ||
			output[index].UpdatedBy != "brief-worker" {
			t.Errorf("output[%d] metadata = %#v, want complete persisted metadata", index, output[index])
		}
		if len(output[index].Sections) != 2 || len(output[index].Sections[0].Citations) != 2 ||
			output[index].Sections[0].Citations[0].ID != eventID ||
			output[index].Sections[0].Citations[1].ID != sourceID {
			t.Errorf("output[%d] sections = %#v, want ordered canonical citations", index, output[index].Sections)
		}
		if output[index].PublicationWindow.From != "2026-07-11T08:00:00Z" ||
			output[index].EventWindow.From != "2026-07-12T08:00:00Z" {
			t.Errorf("output[%d] input windows = (%#v, %#v), want original UTC windows", index, output[index].PublicationWindow, output[index].EventWindow)
		}
	}
}
