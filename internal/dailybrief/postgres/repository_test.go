package postgres

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryPersistsAndRetrievesCompleteBriefs(t *testing.T) {
	database := openDatabase(t)
	repository, err := NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	sourceID, eventID := persistReferences(t, database)

	stored, err := repository.PersistDailyBrief(t.Context(), briefFixture(sourceID, eventID), "brief-worker")
	if err != nil {
		t.Fatalf("PersistDailyBrief() error = %v", err)
	}
	if !validUUID(stored.ID) || stored.CreatedBy != "brief-worker" || stored.UpdatedBy != "brief-worker" ||
		stored.CreatedAt.IsZero() || !stored.CreatedAt.Equal(stored.UpdatedAt) {
		t.Errorf("stored identity/audit = %#v", stored)
	}
	for _, value := range []time.Time{stored.PublicationWindowStart, stored.PublicationWindowEnd, stored.EventWindowStart, stored.EventWindowEnd} {
		if value.Location() != time.UTC {
			t.Errorf("stored time location = %v, want UTC", value.Location())
		}
	}

	got, err := repository.StoredDailyBriefs(t.Context(), calendar.RegionUnitedStates, stored.CreatedAt, stored.CreatedAt, 1)
	if err != nil {
		t.Fatalf("StoredDailyBriefs() error = %v", err)
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], stored) {
		t.Errorf("StoredDailyBriefs() = %#v, want %#v", got, stored)
	}

	var auditedChildren int
	if err := database.Pool.QueryRow(t.Context(), `
SELECT
    (SELECT count(*) FROM daily_brief_sections WHERE created_by = 'brief-worker' AND updated_by = 'brief-worker')
  + (SELECT count(*) FROM daily_brief_citations WHERE created_by = 'brief-worker' AND updated_by = 'brief-worker')
`).Scan(&auditedChildren); err != nil {
		t.Fatalf("query child audit: %v", err)
	}
	if auditedChildren != 5 {
		t.Errorf("audited child count = %d, want 5", auditedChildren)
	}
}

func TestRepositoryRetrievalFiltersOrdersAndLimits(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	sourceID, eventID := persistReferences(t, database)

	briefs := make([]dailybrief.StoredBrief, 0, 3)
	for _, region := range []calendar.Region{calendar.RegionUnitedStates, calendar.RegionUnitedStates, calendar.RegionEurozone} {
		brief := briefFixture(sourceID, eventID)
		brief.Region = region
		stored, err := repository.PersistDailyBrief(t.Context(), brief, "brief-worker")
		if err != nil {
			t.Fatalf("PersistDailyBrief(%q) error = %v", region, err)
		}
		briefs = append(briefs, stored)
	}

	tieTime := time.Date(2026, time.July, 11, 22, 0, 0, 0, time.UTC)
	if _, err := database.Pool.Exec(t.Context(), `UPDATE daily_briefs SET created_at = $1, updated_at = $1 WHERE id = ANY($2::uuid[])`, tieTime, []string{briefs[0].ID, briefs[1].ID, briefs[2].ID}); err != nil {
		t.Fatalf("set deterministic creation times: %v", err)
	}
	wantIDs := []string{briefs[0].ID, briefs[1].ID}
	sort.Strings(wantIDs)

	limited, err := repository.StoredDailyBriefs(t.Context(), calendar.RegionUnitedStates, tieTime.In(time.FixedZone("CEST", 2*60*60)), tieTime, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != wantIDs[0] {
		t.Fatalf("limited StoredDailyBriefs() = (%#v, %v), want first ID %q", limited, err, wantIDs[0])
	}
	all, err := repository.StoredDailyBriefs(t.Context(), calendar.RegionUnitedStates, tieTime, tieTime, 10)
	if err != nil || len(all) != 2 || all[0].ID != wantIDs[0] || all[1].ID != wantIDs[1] {
		t.Fatalf("ordered StoredDailyBriefs() = (%#v, %v), want IDs %v", all, err, wantIDs)
	}
}

func TestRepositoryRollsBackAtomicPersistence(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	brief := briefFixture("00000000-0000-0000-0000-000000000099", "00000000-0000-0000-0000-000000000098")
	if _, err := repository.PersistDailyBrief(t.Context(), brief, "brief-worker"); err == nil || !strings.Contains(err.Error(), "insert daily brief section 0 citation 0") {
		t.Fatalf("PersistDailyBrief() error = %v, want contextual foreign-key failure", err)
	}
	for _, table := range []string{"daily_briefs", "daily_brief_sections", "daily_brief_citations"} {
		var count int
		if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s count = %d, want rollback to zero", table, count)
		}
	}
}

func TestRepositoryRejectsNonCanonicalCitationMetadata(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	sourceID, eventID := persistReferences(t, database)
	valid := briefFixture(sourceID, eventID)

	for _, test := range []struct {
		name   string
		update func(*dailybrief.Citation)
	}{
		{name: "mismatched source", update: func(citation *dailybrief.Citation) { citation.Source = "fabricated-source" }},
		{name: "mismatched URL", update: func(citation *dailybrief.Citation) { citation.URL = "https://attacker.example/fabricated" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			brief := withBrief(valid, func(brief *dailybrief.Brief) { test.update(&brief.Sections[0].Citations[0]) })
			if _, err := repository.PersistDailyBrief(t.Context(), brief, "brief-worker"); err == nil || !strings.Contains(err.Error(), "insert daily brief section 0 citation 0") {
				t.Fatalf("PersistDailyBrief() error = %v, want canonical citation mismatch", err)
			}
			var count int
			if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM daily_briefs").Scan(&count); err != nil || count != 0 {
				t.Fatalf("daily brief count = %d, query error = %v; want zero", count, err)
			}
		})
	}
}

func TestRepositoryValidatesBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	valid := briefFixture("00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002")
	tests := []struct {
		name  string
		brief dailybrief.Brief
		actor string
	}{
		{name: "unsupported region", brief: withBrief(valid, func(brief *dailybrief.Brief) { brief.Region = "asia" }), actor: "worker"},
		{name: "reversed publication window", brief: withBrief(valid, func(brief *dailybrief.Brief) {
			brief.PublicationWindowEnd = brief.PublicationWindowStart.Add(-time.Second)
		}), actor: "worker"},
		{name: "missing provider", brief: withBrief(valid, func(brief *dailybrief.Brief) { brief.Provider = " " }), actor: "worker"},
		{name: "missing sections", brief: withBrief(valid, func(brief *dailybrief.Brief) { brief.Sections = nil }), actor: "worker"},
		{name: "invalid citation UUID", brief: withBrief(valid, func(brief *dailybrief.Brief) { brief.Sections[0].Citations[0].ID = "not-a-uuid" }), actor: "worker"},
		{name: "invalid citation URL", brief: withBrief(valid, func(brief *dailybrief.Brief) { brief.Sections[0].Citations[0].URL = "/relative" }), actor: "worker"},
		{name: "missing actor", brief: valid, actor: " "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.PersistDailyBrief(t.Context(), test.brief, test.actor); err == nil {
				t.Fatal("PersistDailyBrief() error = nil, want validation error")
			}
		})
	}

	queryTests := []struct {
		region calendar.Region
		from   time.Time
		to     time.Time
		limit  int
	}{
		{region: "asia", from: time.Now(), to: time.Now(), limit: 1},
		{region: calendar.RegionUnitedStates, to: time.Now(), limit: 1},
		{region: calendar.RegionUnitedStates, from: time.Now(), limit: 0},
		{region: calendar.RegionUnitedStates, from: time.Now(), to: time.Now(), limit: MaxStoredBriefsLimit + 1},
	}
	for _, test := range queryTests {
		if _, err := repository.StoredDailyBriefs(t.Context(), test.region, test.from, test.to, test.limit); err == nil {
			t.Fatal("StoredDailyBriefs() error = nil, want validation error")
		}
	}
}

func TestNewRepositoryRequiresPostgreSQL(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository() error = nil, want missing database error")
	}
}

func briefFixture(sourceID, eventID string) dailybrief.Brief {
	return dailybrief.Brief{
		Region:                 calendar.RegionUnitedStates,
		PublicationWindowStart: time.Date(2026, time.July, 10, 0, 0, 0, 0, time.FixedZone("CEST", 2*60*60)),
		PublicationWindowEnd:   time.Date(2026, time.July, 11, 0, 0, 0, 0, time.FixedZone("CEST", 2*60*60)),
		EventWindowStart:       time.Date(2026, time.July, 11, 0, 0, 0, 0, time.FixedZone("EDT", -4*60*60)),
		EventWindowEnd:         time.Date(2026, time.July, 12, 0, 0, 0, 0, time.FixedZone("EDT", -4*60*60)),
		Provider:               "openai", Model: "test-model",
		Sections: []dailybrief.Section{
			{Heading: "What matters", Content: "A cited development.", Citations: []dailybrief.Citation{
				{Kind: dailybrief.CitationSourceRecord, ID: sourceID, Source: "example-news", URL: "https://example.com/news/brief-source"},
				{Kind: dailybrief.CitationUpcomingEvent, ID: eventID, Source: "official-calendar", URL: "https://example.com/calendar/brief-event"},
			}},
			{Heading: "What to watch", Content: "The next event.", Citations: []dailybrief.Citation{
				{Kind: dailybrief.CitationUpcomingEvent, ID: eventID, Source: "official-calendar", URL: "https://example.com/calendar/brief-event"},
			}},
		},
	}
}

func persistReferences(t *testing.T, database postgrestest.Database) (string, string) {
	t.Helper()
	sourceRepository, _ := ingestionpostgres.NewRepository(database.Pool)
	source, err := sourceRepository.UpsertSourceRecord(t.Context(), ingestion.SourceRecord{
		Source: "example-news", SourceItemID: "brief-source", OriginalURL: "https://example.com/news/brief-source", Title: "Brief source",
		PublishedAt: time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC), RetrievedAt: time.Date(2026, time.July, 11, 9, 0, 0, 0, time.UTC),
	}, "rss-ingestion")
	if err != nil {
		t.Fatalf("UpsertSourceRecord() error = %v", err)
	}
	eventRepository, _ := calendarpostgres.NewRepository(database.Pool)
	event, err := eventRepository.UpsertEvent(t.Context(), calendar.Event{
		Source: "official-calendar", ExternalEventID: "brief-event", Name: "Brief event", Region: calendar.RegionUnitedStates,
		Type: calendar.EventTypeGDP, ScheduledAt: time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC),
		SourceURL: "https://example.com/calendar/brief-event", RetrievedAt: time.Date(2026, time.July, 11, 9, 0, 0, 0, time.UTC),
	}, "calendar-ingestion")
	if err != nil {
		t.Fatalf("UpsertEvent() error = %v", err)
	}
	return source.ID, event.ID
}

func openDatabase(t *testing.T) postgrestest.Database {
	t.Helper()
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return database
}

func withBrief(brief dailybrief.Brief, update func(*dailybrief.Brief)) dailybrief.Brief {
	brief.Sections = append([]dailybrief.Section(nil), brief.Sections...)
	for index := range brief.Sections {
		brief.Sections[index].Citations = append([]dailybrief.Citation(nil), brief.Sections[index].Citations...)
	}
	update(&brief)
	return brief
}

type panicDB struct{}

func (panicDB) Begin(context.Context) (pgx.Tx, error) {
	panic("validation must happen before beginning a transaction")
}

func (panicDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("validation must happen before querying PostgreSQL")
}
