package postgres_test

import (
	"sort"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

func TestRepositoryRecentSourceRecordsFiltersOrdersAndLimits(t *testing.T) {
	pool := openTestPool(t)
	repository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	windowStart := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(4 * time.Hour)
	records := []ingestion.SourceRecord{
		newSourceRecord("before", windowStart.Add(-time.Microsecond)), newSourceRecord("start", windowStart),
		newSourceRecord("middle-b", windowStart.Add(2*time.Hour)), newSourceRecord("middle-a", windowStart.Add(2*time.Hour)),
		newSourceRecord("end", windowEnd), newSourceRecord("after", windowEnd.Add(time.Microsecond)),
	}
	storedBySourceItemID := make(map[string]ingestion.StoredSourceRecord, len(records))
	for _, record := range records {
		stored, upsertErr := repository.UpsertSourceRecord(t.Context(), record, "rss-ingestion")
		if upsertErr != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", record.SourceItemID, upsertErr)
		}
		storedBySourceItemID[record.SourceItemID] = stored
	}
	got, err := repository.RecentSourceRecords(t.Context(),
		windowStart.In(time.FixedZone("CEST", 2*60*60)), windowEnd, 3)
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
		name                   string
		windowStart, windowEnd time.Time
		limit                  int
	}{
		{name: "missing window start", windowEnd: windowEnd, limit: 1},
		{name: "missing window end", windowStart: windowStart, limit: 1},
		{name: "reversed window", windowStart: windowEnd, windowEnd: windowStart, limit: 1},
		{name: "zero limit", windowStart: windowStart, windowEnd: windowEnd},
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
