package postgres_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
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
	results := make(chan ingestion.StoredSourceRecord, retries)
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

func TestRepositoryRecentSourceRecordsFiltersOrdersAndLimits(t *testing.T) {
	pool := openTestPool(t)
	repository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	windowStart := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(4 * time.Hour)
	records := []ingestion.SourceRecord{
		newSourceRecord("before", windowStart.Add(-time.Microsecond)),
		newSourceRecord("start", windowStart),
		newSourceRecord("middle-b", windowStart.Add(2*time.Hour)),
		newSourceRecord("middle-a", windowStart.Add(2*time.Hour)),
		newSourceRecord("end", windowEnd),
		newSourceRecord("after", windowEnd.Add(time.Microsecond)),
	}

	storedBySourceItemID := make(map[string]ingestion.StoredSourceRecord, len(records))
	for _, record := range records {
		stored, upsertErr := repository.UpsertSourceRecord(t.Context(), record, "rss-ingestion")
		if upsertErr != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", record.SourceItemID, upsertErr)
		}
		storedBySourceItemID[record.SourceItemID] = stored
	}

	got, err := repository.RecentSourceRecords(
		t.Context(),
		windowStart.In(time.FixedZone("CEST", 2*60*60)),
		windowEnd,
		3,
	)
	if err != nil {
		t.Fatalf("RecentSourceRecords() error = %v", err)
	}

	middleIDs := []string{storedBySourceItemID["middle-a"].ID, storedBySourceItemID["middle-b"].ID}
	sort.Strings(middleIDs)
	wantIDs := []string{storedBySourceItemID["end"].ID, middleIDs[0], middleIDs[1]}
	if len(got) != len(wantIDs) {
		t.Fatalf("RecentSourceRecords() count = %d, want %d", len(got), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		if got[index].ID != wantID {
			t.Errorf("RecentSourceRecords()[%d].ID = %q, want %q", index, got[index].ID, wantID)
		}
		want := storedBySourceItemID[got[index].SourceItemID]
		if got[index] != want {
			t.Errorf("RecentSourceRecords()[%d] = %#v, want stored record %#v", index, got[index], want)
		}
	}

	all, err := repository.RecentSourceRecords(t.Context(), windowStart, windowEnd, 10)
	if err != nil {
		t.Fatalf("inclusive RecentSourceRecords() error = %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("inclusive RecentSourceRecords() count = %d, want 4", len(all))
	}
	if all[0].SourceItemID != "end" || all[len(all)-1].SourceItemID != "start" {
		t.Errorf("inclusive RecentSourceRecords() boundary records = (%q, %q), want (end, start)", all[0].SourceItemID, all[len(all)-1].SourceItemID)
	}
}

func TestRepositoryValidatesRecentSourceRecordsQuery(t *testing.T) {
	repository, err := ingestionpostgres.NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	windowStart := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
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
		{name: "limit above maximum", windowStart: windowStart, windowEnd: windowEnd, limit: ingestion.MaxRecentSourceRecordsLimit + 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.RecentSourceRecords(t.Context(), test.windowStart, test.windowEnd, test.limit); err == nil {
				t.Fatal("RecentSourceRecords() error = nil, want validation error")
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

	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("apply database migrations: %v", err)
	}
	return database.Pool
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

func newSourceRecord(sourceItemID string, publishedAt time.Time) ingestion.SourceRecord {
	return ingestion.SourceRecord{
		Source:       "example-news",
		SourceItemID: sourceItemID,
		OriginalURL:  "https://example.com/news/" + sourceItemID,
		Title:        "Source record " + sourceItemID,
		PublishedAt:  publishedAt,
		RetrievedAt:  time.Date(2026, time.July, 11, 13, 0, 0, 0, time.UTC),
	}
}

type panicDB struct{}

func (panicDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("validation must happen before querying PostgreSQL")
}

func (panicDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("validation must happen before querying PostgreSQL")
}
