package postgres_test

import (
	"sort"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
)

func TestRepositoryWatchlistEventCandidatesFiltersOrdersAndLimits(t *testing.T) {
	pool := openTestPool(t)
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(4 * time.Hour)
	events := []calendar.Event{
		newEvent("before", calendar.RegionUnitedStates, windowStart.Add(-time.Microsecond)),
		newEvent("start", calendar.RegionUnitedStates, windowStart),
		newEvent("middle-b", calendar.RegionEurozone, windowStart.Add(2*time.Hour)),
		newEvent("middle-a", calendar.RegionUnitedStates, windowStart.Add(2*time.Hour)),
		newEvent("end", calendar.RegionEurozone, windowEnd),
		newEvent("after", calendar.RegionEurozone, windowEnd.Add(time.Microsecond)),
	}

	storedByExternalID := make(map[string]calendar.StoredEvent, len(events))
	for _, event := range events {
		stored, upsertErr := repository.UpsertEvent(t.Context(), event, "calendar-ingestion")
		if upsertErr != nil {
			t.Fatalf("UpsertEvent(%q) error = %v", event.ExternalEventID, upsertErr)
		}
		storedByExternalID[event.ExternalEventID] = stored
	}

	got, err := repository.WatchlistEventCandidates(
		t.Context(),
		windowStart.In(time.FixedZone("CEST", 2*60*60)),
		windowEnd,
		3,
	)
	if err != nil {
		t.Fatalf("WatchlistEventCandidates() error = %v", err)
	}

	middleIDs := []string{storedByExternalID["middle-a"].ID, storedByExternalID["middle-b"].ID}
	sort.Strings(middleIDs)
	wantIDs := []string{storedByExternalID["start"].ID, middleIDs[0], middleIDs[1]}
	if len(got) != len(wantIDs) {
		t.Fatalf("WatchlistEventCandidates() count = %d, want %d", len(got), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		if got[index].ID != wantID {
			t.Errorf("WatchlistEventCandidates()[%d].ID = %q, want %q", index, got[index].ID, wantID)
		}
		if got[index].Source == "" || got[index].ExternalEventID == "" || got[index].Name == "" ||
			got[index].SourceURL == "" || got[index].CreatedAt.IsZero() || got[index].UpdatedAt.IsZero() ||
			got[index].CreatedBy == "" || got[index].UpdatedBy == "" {
			t.Errorf("WatchlistEventCandidates()[%d] = %#v, want complete canonical record", index, got[index])
		}
		if got[index].ScheduledAt.Location() != time.UTC || got[index].RetrievedAt.Location() != time.UTC ||
			got[index].CreatedAt.Location() != time.UTC || got[index].UpdatedAt.Location() != time.UTC {
			t.Errorf(
				"WatchlistEventCandidates()[%d] time zones = (%v, %v, %v, %v), want UTC",
				index,
				got[index].ScheduledAt.Location(),
				got[index].RetrievedAt.Location(),
				got[index].CreatedAt.Location(),
				got[index].UpdatedAt.Location(),
			)
		}
	}

	all, err := repository.WatchlistEventCandidates(t.Context(), windowStart, windowEnd, 10)
	if err != nil {
		t.Fatalf("inclusive WatchlistEventCandidates() error = %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("inclusive WatchlistEventCandidates() count = %d, want 4", len(all))
	}
	if all[0].ExternalEventID != "start" || all[len(all)-1].ExternalEventID != "end" {
		t.Errorf(
			"inclusive WatchlistEventCandidates() boundary events = (%q, %q), want (start, end)",
			all[0].ExternalEventID,
			all[len(all)-1].ExternalEventID,
		)
	}
}

func TestRepositoryWatchlistEventCandidatesReturnsNonNilEmptyResult(t *testing.T) {
	pool := openTestPool(t)
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	got, err := repository.WatchlistEventCandidates(t.Context(), windowStart, windowStart.Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("WatchlistEventCandidates() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("WatchlistEventCandidates() = %#v, want non-nil empty result", got)
	}
}

func TestRepositoryValidatesWatchlistEventCandidatesQuery(t *testing.T) {
	repository, err := calendarpostgres.NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	tests := []struct {
		name        string
		windowStart time.Time
		windowEnd   time.Time
		limit       int
	}{
		{name: "missing window start", windowEnd: windowEnd, limit: 1},
		{name: "missing window end", windowStart: windowStart, limit: 1},
		{name: "reversed window", windowStart: windowEnd, windowEnd: windowStart, limit: 1},
		{name: "zero limit", windowStart: windowStart, windowEnd: windowEnd, limit: 0},
		{name: "negative limit", windowStart: windowStart, windowEnd: windowEnd, limit: -1},
		{
			name:        "limit above maximum",
			windowStart: windowStart,
			windowEnd:   windowEnd,
			limit:       calendar.MaxWatchlistEventCandidatesLimit + 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.WatchlistEventCandidates(
				t.Context(), test.windowStart, test.windowEnd, test.limit,
			); err == nil {
				t.Fatal("WatchlistEventCandidates() error = nil, want validation error")
			}
		})
	}
}
