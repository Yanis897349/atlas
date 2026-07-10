package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
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
		Source:          "example-calendar",
		ExternalEventID: "us-cpi-2026-07",
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
	retried, err := repository.UpsertEvent(t.Context(), initial, "retry-worker")
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

	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set")
	}

	adminPool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	t.Cleanup(adminPool.Close)
	if err := adminPool.Ping(t.Context()); err != nil {
		t.Fatalf("ping test PostgreSQL: %v", err)
	}

	schema := "atlas_calendar_test_" + randomHex(t, 8)
	if _, err := adminPool.Exec(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("connect to isolated test schema: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := databasepostgres.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("apply database migrations: %v", err)
	}

	return pool
}

func randomHex(t *testing.T, size int) string {
	t.Helper()

	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate test schema name: %v", err)
	}
	return hex.EncodeToString(value)
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
