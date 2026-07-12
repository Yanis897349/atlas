package postgres_test

import (
	"sort"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
)

func TestRepositoryUpcomingEventsFiltersOrdersAndLimits(t *testing.T) {
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
		newEvent("middle-b", calendar.RegionUnitedStates, windowStart.Add(2*time.Hour)),
		newEvent("middle-a", calendar.RegionUnitedStates, windowStart.Add(2*time.Hour)),
		newEvent("end", calendar.RegionUnitedStates, windowEnd),
		newEvent("after", calendar.RegionUnitedStates, windowEnd.Add(time.Microsecond)),
		newEvent("other-region", calendar.RegionEurozone, windowStart.Add(time.Hour)),
	}
	storedByExternalID := make(map[string]calendar.StoredEvent, len(events))
	for _, event := range events {
		stored, upsertErr := repository.UpsertEvent(t.Context(), event, "calendar-ingestion")
		if upsertErr != nil {
			t.Fatalf("UpsertEvent(%q) error = %v", event.ExternalEventID, upsertErr)
		}
		storedByExternalID[event.ExternalEventID] = stored
	}
	got, err := repository.UpcomingEvents(t.Context(), calendar.RegionUnitedStates,
		windowStart.In(time.FixedZone("CEST", 2*60*60)), windowEnd, 3)
	if err != nil {
		t.Fatalf("UpcomingEvents() error = %v", err)
	}
	middleIDs := []string{storedByExternalID["middle-a"].ID, storedByExternalID["middle-b"].ID}
	sort.Strings(middleIDs)
	wantIDs := []string{storedByExternalID["start"].ID, middleIDs[0], middleIDs[1]}
	if len(got) != len(wantIDs) {
		t.Fatalf("UpcomingEvents() count = %d, want %d", len(got), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		if got[index].ID != wantID {
			t.Errorf("UpcomingEvents()[%d].ID = %q, want %q", index, got[index].ID, wantID)
		}
		if got[index].Source == "" || got[index].SourceURL == "" {
			t.Errorf("UpcomingEvents()[%d] source citation = (%q, %q), want both populated", index, got[index].Source, got[index].SourceURL)
		}
		if got[index].ScheduledAt.Location() != time.UTC || got[index].RetrievedAt.Location() != time.UTC ||
			got[index].CreatedAt.Location() != time.UTC || got[index].UpdatedAt.Location() != time.UTC {
			t.Errorf("UpcomingEvents()[%d] timestamps must use UTC", index)
		}
	}
	all, err := repository.UpcomingEvents(t.Context(), calendar.RegionUnitedStates, windowStart, windowEnd, 10)
	if err != nil {
		t.Fatalf("inclusive UpcomingEvents() error = %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("inclusive UpcomingEvents() count = %d, want 4", len(all))
	}
	if all[0].ExternalEventID != "start" || all[len(all)-1].ExternalEventID != "end" {
		t.Errorf("inclusive UpcomingEvents() boundary events = (%q, %q), want (start, end)", all[0].ExternalEventID, all[len(all)-1].ExternalEventID)
	}
}

func TestRepositoryValidatesUpcomingEventsQuery(t *testing.T) {
	repository, err := calendarpostgres.NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	tests := []struct {
		name                   string
		region                 calendar.Region
		windowStart, windowEnd time.Time
		limit                  int
	}{
		{name: "unsupported region", region: "asia", windowStart: windowStart, windowEnd: windowEnd, limit: 1},
		{name: "missing window start", region: calendar.RegionUnitedStates, windowEnd: windowEnd, limit: 1},
		{name: "missing window end", region: calendar.RegionUnitedStates, windowStart: windowStart, limit: 1},
		{name: "reversed window", region: calendar.RegionUnitedStates, windowStart: windowEnd, windowEnd: windowStart, limit: 1},
		{name: "zero limit", region: calendar.RegionUnitedStates, windowStart: windowStart, windowEnd: windowEnd},
		{name: "negative limit", region: calendar.RegionUnitedStates, windowStart: windowStart, windowEnd: windowEnd, limit: -1},
		{name: "limit above maximum", region: calendar.RegionUnitedStates, windowStart: windowStart, windowEnd: windowEnd, limit: calendar.MaxUpcomingEventsLimit + 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.UpcomingEvents(t.Context(), test.region, test.windowStart, test.windowEnd, test.limit); err == nil {
				t.Fatal("UpcomingEvents() error = nil, want validation error")
			}
		})
	}
}
