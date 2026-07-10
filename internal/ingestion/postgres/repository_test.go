package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"testing"
	"time"

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryUpsertSourceRecordIsIdempotent(t *testing.T) {
	pool := openTestPool(t)
	repository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	initial := ingestion.SourceRecord{
		Source:       "example-central-bank",
		SourceItemID: "stable-item-id",
		OriginalURL:  "https://example.com/releases/rates",
		Title:        "Policy rate unchanged",
		PublishedAt:  time.Date(2026, time.July, 9, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60)),
		RetrievedAt:  time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC),
	}

	created, err := repository.UpsertSourceRecord(t.Context(), initial, "rss-ingestion")
	if err != nil {
		t.Fatalf("first UpsertSourceRecord() error = %v", err)
	}
	retried, err := repository.UpsertSourceRecord(t.Context(), initial, "retry-worker")
	if err != nil {
		t.Fatalf("retry UpsertSourceRecord() error = %v", err)
	}

	if retried != created {
		t.Errorf("retry record = %#v, want unchanged %#v", retried, created)
	}
	assertSourceRecordCount(t, pool, 1)

	older := initial
	older.Title = "Stale title"
	older.RetrievedAt = initial.RetrievedAt.Add(-time.Minute)
	unchanged, err := repository.UpsertSourceRecord(t.Context(), older, "older-worker")
	if err != nil {
		t.Fatalf("older UpsertSourceRecord() error = %v", err)
	}
	if unchanged != created {
		t.Errorf("older record = %#v, want unchanged %#v", unchanged, created)
	}

	newer := initial
	newer.OriginalURL = "https://example.com/releases/rates-corrected"
	newer.Title = "Policy rate unchanged — corrected"
	newer.PublishedAt = initial.PublishedAt.Add(time.Minute)
	newer.RetrievedAt = initial.RetrievedAt.Add(time.Hour)
	updated, err := repository.UpsertSourceRecord(t.Context(), newer, "correction-worker")
	if err != nil {
		t.Fatalf("newer UpsertSourceRecord() error = %v", err)
	}

	if updated.ID != created.ID {
		t.Errorf("updated ID = %q, want %q", updated.ID, created.ID)
	}
	if updated.CreatedAt != created.CreatedAt || updated.CreatedBy != created.CreatedBy {
		t.Errorf("creation audit changed from (%v, %q) to (%v, %q)", created.CreatedAt, created.CreatedBy, updated.CreatedAt, updated.CreatedBy)
	}
	if updated.OriginalURL != newer.OriginalURL || updated.Title != newer.Title {
		t.Errorf("updated metadata = (%q, %q), want (%q, %q)", updated.OriginalURL, updated.Title, newer.OriginalURL, newer.Title)
	}
	if !updated.PublishedAt.Equal(newer.PublishedAt) || !updated.RetrievedAt.Equal(newer.RetrievedAt) {
		t.Errorf("updated times = (%v, %v), want (%v, %v)", updated.PublishedAt, updated.RetrievedAt, newer.PublishedAt, newer.RetrievedAt)
	}
	if updated.UpdatedBy != "correction-worker" {
		t.Errorf("UpdatedBy = %q, want correction worker", updated.UpdatedBy)
	}
	assertSourceRecordCount(t, pool, 1)
}

func TestRepositoryUpsertSourceRecordHandlesConcurrentRetries(t *testing.T) {
	pool := openTestPool(t)
	repository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	record := ingestion.SourceRecord{
		Source:       "example-central-bank",
		SourceItemID: "concurrent-item-id",
		OriginalURL:  "https://example.com/releases/concurrent",
		Title:        "Concurrent source record",
		PublishedAt:  time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC),
		RetrievedAt:  time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC),
	}

	const retries = 8
	results := make(chan ingestionpostgres.StoredSourceRecord, retries)
	errors := make(chan error, retries)
	var waitGroup sync.WaitGroup
	for range retries {
		waitGroup.Go(func() {
			stored, err := repository.UpsertSourceRecord(t.Context(), record, "rss-ingestion")
			if err != nil {
				errors <- err
				return
			}
			results <- stored
		})
	}
	waitGroup.Wait()
	close(results)
	close(errors)

	for err := range errors {
		t.Errorf("concurrent UpsertSourceRecord() error = %v", err)
	}
	var storedID string
	for stored := range results {
		if storedID == "" {
			storedID = stored.ID
		}
		if stored.ID != storedID {
			t.Errorf("concurrent record ID = %q, want %q", stored.ID, storedID)
		}
	}
	assertSourceRecordCount(t, pool, 1)
}

func TestRepositoryValidatesSourceRecord(t *testing.T) {
	repository, err := ingestionpostgres.NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	valid := ingestion.SourceRecord{
		Source:       "source",
		SourceItemID: "item",
		OriginalURL:  "https://example.com/item",
		Title:        "Title",
		PublishedAt:  time.Now(),
		RetrievedAt:  time.Now(),
	}
	tests := []struct {
		name   string
		record ingestion.SourceRecord
		actor  string
	}{
		{name: "missing source", record: withField(valid, func(record *ingestion.SourceRecord) { record.Source = "" }), actor: "worker"},
		{name: "missing item ID", record: withField(valid, func(record *ingestion.SourceRecord) { record.SourceItemID = "" }), actor: "worker"},
		{name: "invalid URL", record: withField(valid, func(record *ingestion.SourceRecord) { record.OriginalURL = "/item" }), actor: "worker"},
		{name: "missing title", record: withField(valid, func(record *ingestion.SourceRecord) { record.Title = " " }), actor: "worker"},
		{name: "missing published time", record: withField(valid, func(record *ingestion.SourceRecord) { record.PublishedAt = time.Time{} }), actor: "worker"},
		{name: "missing retrieved time", record: withField(valid, func(record *ingestion.SourceRecord) { record.RetrievedAt = time.Time{} }), actor: "worker"},
		{name: "missing actor", record: valid, actor: " "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.UpsertSourceRecord(t.Context(), test.record, test.actor); err == nil {
				t.Fatal("UpsertSourceRecord() error = nil, want validation error")
			}
		})
	}
}

func TestNewRepositoryRequiresDatabase(t *testing.T) {
	if _, err := ingestionpostgres.NewRepository(nil); err == nil {
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

	schema := "atlas_test_" + randomHex(t, 8)
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

func assertSourceRecordCount(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()

	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM source_records").Scan(&count); err != nil {
		t.Fatalf("count source records: %v", err)
	}
	if count != want {
		t.Errorf("source record count = %d, want %d", count, want)
	}
}

func withField(record ingestion.SourceRecord, update func(*ingestion.SourceRecord)) ingestion.SourceRecord {
	update(&record)
	return record
}

type panicDB struct{}

func (panicDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("validation must happen before querying PostgreSQL")
}
