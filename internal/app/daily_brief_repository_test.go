package app

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/jackc/pgx/v5"
)

func TestDailyBriefRepositoryPersistsAndRetrievesCompleteBriefs(t *testing.T) {
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository, err := newDailyBriefRepository(database.Pool)
	if err != nil {
		t.Fatalf("newDailyBriefRepository() error = %v", err)
	}

	sourceRepository, _ := ingestionpostgres.NewRepository(database.Pool)
	source, err := sourceRepository.UpsertSourceRecord(t.Context(), ingestion.SourceRecord{
		Source:       "example-news",
		SourceItemID: "brief-source",
		OriginalURL:  "https://example.com/news/brief-source",
		Title:        "Brief source",
		PublishedAt:  time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC),
		RetrievedAt:  time.Date(2026, time.July, 11, 9, 0, 0, 0, time.UTC),
	}, "rss-ingestion")
	if err != nil {
		t.Fatalf("UpsertSourceRecord() error = %v", err)
	}
	eventRepository, _ := calendarpostgres.NewRepository(database.Pool)
	event, err := eventRepository.UpsertEvent(t.Context(), calendar.Event{
		Source:          "official-calendar",
		ExternalEventID: "brief-event",
		Name:            "Brief event",
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeGDP,
		ScheduledAt:     time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC),
		SourceURL:       "https://example.com/calendar/brief-event",
		RetrievedAt:     time.Date(2026, time.July, 11, 9, 0, 0, 0, time.UTC),
	}, "calendar-ingestion")
	if err != nil {
		t.Fatalf("UpsertEvent() error = %v", err)
	}

	brief := persistedDailyBriefFixture(source.ID, event.ID)
	stored, err := repository.PersistDailyBrief(t.Context(), brief, "brief-worker")
	if err != nil {
		t.Fatalf("PersistDailyBrief() error = %v", err)
	}
	if !validUUID(stored.ID) {
		t.Errorf("stored ID = %q, want UUID", stored.ID)
	}
	if stored.CreatedBy != "brief-worker" || stored.UpdatedBy != "brief-worker" ||
		stored.CreatedAt.IsZero() || !stored.CreatedAt.Equal(stored.UpdatedAt) {
		t.Errorf("stored audit = (%v, %v, %q, %q), want creation audit", stored.CreatedAt, stored.UpdatedAt, stored.CreatedBy, stored.UpdatedBy)
	}
	for _, value := range []time.Time{
		stored.publicationWindowStart,
		stored.publicationWindowEnd,
		stored.eventWindowStart,
		stored.eventWindowEnd,
	} {
		if value.Location() != time.UTC {
			t.Errorf("stored time location = %v, want UTC", value.Location())
		}
	}

	got, err := repository.StoredDailyBriefs(
		t.Context(),
		calendar.RegionUnitedStates,
		stored.CreatedAt,
		stored.CreatedAt,
		1,
	)
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

func TestDailyBriefRepositoryRetrievalFiltersOrdersAndLimits(t *testing.T) {
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository, _ := newDailyBriefRepository(database.Pool)
	sourceID, eventID := persistDailyBriefReferences(t, database)

	briefs := make([]storedDailyBrief, 0, 3)
	for _, region := range []calendar.Region{
		calendar.RegionUnitedStates,
		calendar.RegionUnitedStates,
		calendar.RegionEurozone,
	} {
		brief := persistedDailyBriefFixture(sourceID, eventID)
		brief.region = region
		stored, err := repository.PersistDailyBrief(t.Context(), brief, "brief-worker")
		if err != nil {
			t.Fatalf("PersistDailyBrief(%q) error = %v", region, err)
		}
		briefs = append(briefs, stored)
	}

	tieTime := time.Date(2026, time.July, 11, 22, 0, 0, 0, time.UTC)
	if _, err := database.Pool.Exec(
		t.Context(),
		`UPDATE daily_briefs SET created_at = $1, updated_at = $1 WHERE id = ANY($2::uuid[])`,
		tieTime,
		[]string{briefs[0].ID, briefs[1].ID, briefs[2].ID},
	); err != nil {
		t.Fatalf("set deterministic creation times: %v", err)
	}

	got, err := repository.StoredDailyBriefs(
		t.Context(),
		calendar.RegionUnitedStates,
		tieTime.In(time.FixedZone("CEST", 2*60*60)),
		tieTime,
		1,
	)
	if err != nil {
		t.Fatalf("StoredDailyBriefs() error = %v", err)
	}
	wantIDs := []string{briefs[0].ID, briefs[1].ID}
	sort.Strings(wantIDs)
	if len(got) != 1 || got[0].ID != wantIDs[0] {
		t.Errorf("limited StoredDailyBriefs() = %#v, want first ID %q", got, wantIDs[0])
	}

	got, err = repository.StoredDailyBriefs(
		t.Context(),
		calendar.RegionUnitedStates,
		tieTime,
		tieTime,
		10,
	)
	if err != nil {
		t.Fatalf("inclusive StoredDailyBriefs() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != wantIDs[0] || got[1].ID != wantIDs[1] {
		t.Errorf("ordered StoredDailyBriefs() IDs = %#v, want %v", []string{got[0].ID, got[1].ID}, wantIDs)
	}
}

func TestDailyBriefRepositoryRollsBackAtomicPersistence(t *testing.T) {
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository, _ := newDailyBriefRepository(database.Pool)
	brief := persistedDailyBriefFixture(
		"00000000-0000-0000-0000-000000000099",
		"00000000-0000-0000-0000-000000000098",
	)

	if _, err := repository.PersistDailyBrief(t.Context(), brief, "brief-worker"); err == nil ||
		!strings.Contains(err.Error(), "insert daily brief section 0 citation 0") {
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

func TestDailyBriefRepositoryRejectsNonCanonicalCitationMetadata(t *testing.T) {
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository, _ := newDailyBriefRepository(database.Pool)
	sourceID, eventID := persistDailyBriefReferences(t, database)
	valid := persistedDailyBriefFixture(sourceID, eventID)

	for _, test := range []struct {
		name   string
		update func(*dailyBriefCitation)
	}{
		{name: "mismatched source", update: func(citation *dailyBriefCitation) { citation.source = "fabricated-source" }},
		{name: "mismatched URL", update: func(citation *dailyBriefCitation) { citation.url = "https://attacker.example/fabricated" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			brief := withPersistedBrief(valid, func(brief *dailyBrief) {
				test.update(&brief.sections[0].citations[0])
			})
			if _, err := repository.PersistDailyBrief(t.Context(), brief, "brief-worker"); err == nil ||
				!strings.Contains(err.Error(), "insert daily brief section 0 citation 0") {
				t.Fatalf("PersistDailyBrief() error = %v, want canonical citation mismatch", err)
			}

			var count int
			if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM daily_briefs").Scan(&count); err != nil {
				t.Fatalf("count daily briefs: %v", err)
			}
			if count != 0 {
				t.Errorf("daily brief count = %d, want mismatched citation rollback", count)
			}
		})
	}
}

func TestDailyBriefRepositoryValidatesBeforePostgreSQL(t *testing.T) {
	repository, err := newDailyBriefRepository(panicDailyBriefDB{})
	if err != nil {
		t.Fatalf("newDailyBriefRepository() error = %v", err)
	}
	valid := persistedDailyBriefFixture(
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000002",
	)
	tests := []struct {
		name  string
		brief dailyBrief
		actor string
	}{
		{name: "unsupported region", brief: withPersistedBrief(valid, func(brief *dailyBrief) { brief.region = "asia" }), actor: "worker"},
		{name: "reversed publication window", brief: withPersistedBrief(valid, func(brief *dailyBrief) { brief.publicationWindowEnd = brief.publicationWindowStart.Add(-time.Second) }), actor: "worker"},
		{name: "missing provider", brief: withPersistedBrief(valid, func(brief *dailyBrief) { brief.provider = " " }), actor: "worker"},
		{name: "missing sections", brief: withPersistedBrief(valid, func(brief *dailyBrief) { brief.sections = nil }), actor: "worker"},
		{name: "invalid citation UUID", brief: withPersistedBrief(valid, func(brief *dailyBrief) { brief.sections[0].citations[0].id = "not-a-uuid" }), actor: "worker"},
		{name: "invalid citation URL", brief: withPersistedBrief(valid, func(brief *dailyBrief) { brief.sections[0].citations[0].url = "/relative" }), actor: "worker"},
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
		name   string
		region calendar.Region
		from   time.Time
		to     time.Time
		limit  int
	}{
		{name: "unsupported region", region: "asia", from: time.Now(), to: time.Now(), limit: 1},
		{name: "missing start", region: calendar.RegionUnitedStates, to: time.Now(), limit: 1},
		{name: "missing end", region: calendar.RegionUnitedStates, from: time.Now(), limit: 1},
		{name: "reversed window", region: calendar.RegionUnitedStates, from: time.Now(), to: time.Now().Add(-time.Hour), limit: 1},
		{name: "zero limit", region: calendar.RegionUnitedStates, from: time.Now(), to: time.Now(), limit: 0},
		{name: "limit above maximum", region: calendar.RegionUnitedStates, from: time.Now(), to: time.Now(), limit: maxStoredDailyBriefsLimit + 1},
	}
	for _, test := range queryTests {
		t.Run("query "+test.name, func(t *testing.T) {
			if _, err := repository.StoredDailyBriefs(t.Context(), test.region, test.from, test.to, test.limit); err == nil {
				t.Fatal("StoredDailyBriefs() error = nil, want validation error")
			}
		})
	}
}

func TestNewDailyBriefRepositoryRequiresPostgreSQL(t *testing.T) {
	if _, err := newDailyBriefRepository(nil); err == nil {
		t.Fatal("newDailyBriefRepository() error = nil, want missing database error")
	}
}

func persistedDailyBriefFixture(sourceID, eventID string) dailyBrief {
	publicationStart := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	eventStart := publicationStart.Add(24 * time.Hour)
	return dailyBrief{
		region:                 calendar.RegionUnitedStates,
		publicationWindowStart: publicationStart,
		publicationWindowEnd:   publicationStart.Add(12 * time.Hour),
		eventWindowStart:       eventStart,
		eventWindowEnd:         eventStart.Add(48 * time.Hour),
		provider:               "openai",
		model:                  "test-model",
		sections: []dailyBriefSection{
			{
				heading: "First section",
				content: "First content.",
				citations: []dailyBriefCitation{
					{kind: dailyBriefCitationUpcomingEvent, id: eventID, source: "official-calendar", url: "https://example.com/calendar/brief-event"},
					{kind: dailyBriefCitationSourceRecord, id: sourceID, source: "example-news", url: "https://example.com/news/brief-source"},
				},
			},
			{
				heading: "Second section",
				content: "Second content.",
				citations: []dailyBriefCitation{
					{kind: dailyBriefCitationSourceRecord, id: sourceID, source: "example-news", url: "https://example.com/news/brief-source"},
				},
			},
		},
	}
}

func persistDailyBriefReferences(t *testing.T, database postgrestest.Database) (string, string) {
	t.Helper()
	sourceRepository, _ := ingestionpostgres.NewRepository(database.Pool)
	source, err := sourceRepository.UpsertSourceRecord(t.Context(), ingestion.SourceRecord{
		Source: "example-news", SourceItemID: "source", OriginalURL: "https://example.com/news/brief-source",
		Title: "Source", PublishedAt: time.Now(), RetrievedAt: time.Now(),
	}, "worker")
	if err != nil {
		t.Fatalf("persist source: %v", err)
	}
	eventRepository, _ := calendarpostgres.NewRepository(database.Pool)
	event, err := eventRepository.UpsertEvent(t.Context(), calendar.Event{
		Source: "official-calendar", ExternalEventID: "event", Name: "Event",
		Region: calendar.RegionUnitedStates, Type: calendar.EventTypeGDP, ScheduledAt: time.Now().Add(time.Hour),
		SourceURL: "https://example.com/calendar/brief-event", RetrievedAt: time.Now(),
	}, "worker")
	if err != nil {
		t.Fatalf("persist event: %v", err)
	}
	return source.ID, event.ID
}

func withPersistedBrief(brief dailyBrief, update func(*dailyBrief)) dailyBrief {
	brief.sections = append([]dailyBriefSection(nil), brief.sections...)
	for index := range brief.sections {
		brief.sections[index].citations = append([]dailyBriefCitation(nil), brief.sections[index].citations...)
	}
	update(&brief)
	return brief
}

type panicDailyBriefDB struct{}

func (panicDailyBriefDB) Begin(context.Context) (pgx.Tx, error) {
	panic("validation must happen before beginning a transaction")
}

func (panicDailyBriefDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("validation must happen before querying PostgreSQL")
}
