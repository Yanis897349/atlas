package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryUpsertEventIsIdempotent(t *testing.T) {
	pool := openTestPool(t)
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	initial := calendar.Event{
		Source:          "  example-calendar ",
		ExternalEventID: " us-cpi-2026-07\t",
		Name:            "Consumer Price Index",
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeInflation,
		ScheduledAt:     time.Date(2026, time.August, 12, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60)),
		SourceURL:       "https://example.com/calendar/us-cpi",
		RetrievedAt:     time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC),
	}

	created, err := repository.UpsertEvent(t.Context(), initial, "calendar-ingestion")
	if err != nil {
		t.Fatalf("first UpsertEvent() error = %v", err)
	}
	if created.Source != "example-calendar" || created.ExternalEventID != "us-cpi-2026-07" {
		t.Errorf("stored identity = (%q, %q), want normalized identity", created.Source, created.ExternalEventID)
	}

	retry := initial
	retry.Source = "example-calendar"
	retry.ExternalEventID = "us-cpi-2026-07"
	retried, err := repository.UpsertEvent(t.Context(), retry, "retry-worker")
	if err != nil {
		t.Fatalf("retry UpsertEvent() error = %v", err)
	}

	if retried != created {
		t.Errorf("retry event = %#v, want unchanged %#v", retried, created)
	}
	assertEventCount(t, pool, 1)

	older := initial
	older.Name = "Stale event name"
	older.RetrievedAt = initial.RetrievedAt.Add(-time.Minute)
	unchanged, err := repository.UpsertEvent(t.Context(), older, "older-worker")
	if err != nil {
		t.Fatalf("older UpsertEvent() error = %v", err)
	}
	if unchanged != created {
		t.Errorf("older event = %#v, want unchanged %#v", unchanged, created)
	}

	newer := initial
	newer.Name = "CPI release"
	newer.Region = calendar.RegionEurozone
	newer.Type = calendar.EventTypeEmployment
	newer.ScheduledAt = initial.ScheduledAt.Add(time.Hour)
	newer.SourceURL = "https://example.com/calendar/corrected-cpi"
	newer.RetrievedAt = initial.RetrievedAt.Add(time.Hour)
	updated, err := repository.UpsertEvent(t.Context(), newer, "correction-worker")
	if err != nil {
		t.Fatalf("newer UpsertEvent() error = %v", err)
	}

	if updated.ID != created.ID {
		t.Errorf("updated ID = %q, want %q", updated.ID, created.ID)
	}
	if updated.Source != created.Source || updated.ExternalEventID != created.ExternalEventID {
		t.Errorf("updated identity = (%q, %q), want (%q, %q)", updated.Source, updated.ExternalEventID, created.Source, created.ExternalEventID)
	}
	if updated.CreatedAt != created.CreatedAt || updated.CreatedBy != created.CreatedBy {
		t.Errorf("creation audit changed from (%v, %q) to (%v, %q)", created.CreatedAt, created.CreatedBy, updated.CreatedAt, updated.CreatedBy)
	}
	if updated.Name != newer.Name || updated.Region != newer.Region || updated.Type != newer.Type {
		t.Errorf("updated classification = (%q, %q, %q), want (%q, %q, %q)", updated.Name, updated.Region, updated.Type, newer.Name, newer.Region, newer.Type)
	}
	if !updated.ScheduledAt.Equal(newer.ScheduledAt) || !updated.RetrievedAt.Equal(newer.RetrievedAt) {
		t.Errorf("updated times = (%v, %v), want (%v, %v)", updated.ScheduledAt, updated.RetrievedAt, newer.ScheduledAt, newer.RetrievedAt)
	}
	if updated.SourceURL != newer.SourceURL || updated.UpdatedBy != "correction-worker" {
		t.Errorf("updated metadata = (%q, %q), want (%q, %q)", updated.SourceURL, updated.UpdatedBy, newer.SourceURL, "correction-worker")
	}
	assertEventCount(t, pool, 1)
}

func TestRepositoryValidatesEvent(t *testing.T) {
	repository, err := calendarpostgres.NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	valid := calendar.Event{
		Source:          "source",
		ExternalEventID: "event",
		Name:            "Event name",
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeGDP,
		ScheduledAt:     time.Now(),
		SourceURL:       "https://example.com/event",
		RetrievedAt:     time.Now(),
	}
	tests := []struct {
		name  string
		event calendar.Event
		actor string
	}{
		{name: "missing source", event: withField(valid, func(event *calendar.Event) { event.Source = "" }), actor: "worker"},
		{name: "missing external event ID", event: withField(valid, func(event *calendar.Event) { event.ExternalEventID = " " }), actor: "worker"},
		{name: "missing name", event: withField(valid, func(event *calendar.Event) { event.Name = "" }), actor: "worker"},
		{name: "unsupported region", event: withField(valid, func(event *calendar.Event) { event.Region = "asia" }), actor: "worker"},
		{name: "unsupported event type", event: withField(valid, func(event *calendar.Event) { event.Type = "other" }), actor: "worker"},
		{name: "missing scheduled time", event: withField(valid, func(event *calendar.Event) { event.ScheduledAt = time.Time{} }), actor: "worker"},
		{name: "invalid source URL", event: withField(valid, func(event *calendar.Event) { event.SourceURL = "/event" }), actor: "worker"},
		{name: "missing retrieved time", event: withField(valid, func(event *calendar.Event) { event.RetrievedAt = time.Time{} }), actor: "worker"},
		{name: "missing actor", event: valid, actor: " "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.UpsertEvent(t.Context(), test.event, test.actor); err == nil {
				t.Fatal("UpsertEvent() error = nil, want validation error")
			}
		})
	}
}

func TestNewRepositoryRequiresDatabase(t *testing.T) {
	if _, err := calendarpostgres.NewRepository(nil); err == nil {
		t.Fatal("NewRepository() error = nil, want missing database error")
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("apply database migrations: %v", err)
	}
	return database.Pool
}

func assertEventCount(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()

	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM economic_events").Scan(&count); err != nil {
		t.Fatalf("count economic events: %v", err)
	}
	if count != want {
		t.Errorf("economic event count = %d, want %d", count, want)
	}
}

func withField(event calendar.Event, update func(*calendar.Event)) calendar.Event {
	update(&event)
	return event
}

type panicDB struct{}

func (panicDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("validation must happen before querying PostgreSQL")
}
