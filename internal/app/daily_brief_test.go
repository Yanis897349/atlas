package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

func TestAssembleDailyBriefInputPreservesRepositoryResults(t *testing.T) {
	publicationStart := time.Date(2026, time.July, 11, 7, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	publicationEnd := publicationStart.Add(4 * time.Hour)
	eventStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	eventEnd := eventStart.Add(24 * time.Hour)
	records := []ingestionpostgres.StoredSourceRecord{
		newStoredSourceRecord("newer", publicationEnd),
		newStoredSourceRecord("older", publicationStart),
	}
	events := []calendarpostgres.StoredEvent{
		newStoredEvent("first", eventStart),
		newStoredEvent("second", eventEnd),
	}
	sourceRepository := &dailyBriefSourceRecordsStub{records: records}
	eventRepository := &dailyBriefEventsStub{events: events}
	query := dailyBriefInputQuery{
		region:                 calendar.RegionUnitedStates,
		publicationWindowStart: publicationStart,
		publicationWindowEnd:   publicationEnd,
		sourceRecordLimit:      12,
		eventWindowStart:       eventStart,
		eventWindowEnd:         eventEnd,
		upcomingEventLimit:     8,
	}

	got, err := assembleDailyBriefInput(t.Context(), sourceRepository, eventRepository, query)
	if err != nil {
		t.Fatalf("assembleDailyBriefInput() error = %v", err)
	}

	want := dailyBriefInput{
		region:                 calendar.RegionUnitedStates,
		publicationWindowStart: publicationStart.UTC(),
		publicationWindowEnd:   publicationEnd.UTC(),
		eventWindowStart:       eventStart.UTC(),
		eventWindowEnd:         eventEnd.UTC(),
		sourceRecords:          records,
		upcomingEvents:         events,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("assembleDailyBriefInput() = %#v, want %#v", got, want)
	}
	if sourceRepository.calls != 1 || sourceRepository.windowStart != publicationStart.UTC() ||
		sourceRepository.windowEnd != publicationEnd.UTC() || sourceRepository.limit != 12 {
		t.Errorf(
			"source repository call = (%d, %v, %v, %d), want (1, %v, %v, 12)",
			sourceRepository.calls,
			sourceRepository.windowStart,
			sourceRepository.windowEnd,
			sourceRepository.limit,
			publicationStart.UTC(),
			publicationEnd.UTC(),
		)
	}
	if eventRepository.calls != 1 || eventRepository.region != calendar.RegionUnitedStates ||
		eventRepository.windowStart != eventStart.UTC() || eventRepository.windowEnd != eventEnd.UTC() ||
		eventRepository.limit != 8 {
		t.Errorf(
			"event repository call = (%d, %q, %v, %v, %d), want (1, %q, %v, %v, 8)",
			eventRepository.calls,
			eventRepository.region,
			eventRepository.windowStart,
			eventRepository.windowEnd,
			eventRepository.limit,
			calendar.RegionUnitedStates,
			eventStart.UTC(),
			eventEnd.UTC(),
		)
	}
}

func TestAssembleDailyBriefInputAcceptsEmptyResultsAndEqualWindows(t *testing.T) {
	query := validDailyBriefInputQuery()
	query.publicationWindowEnd = query.publicationWindowStart
	query.eventWindowEnd = query.eventWindowStart

	got, err := assembleDailyBriefInput(
		t.Context(),
		&dailyBriefSourceRecordsStub{},
		&dailyBriefEventsStub{},
		query,
	)
	if err != nil {
		t.Fatalf("assembleDailyBriefInput() error = %v", err)
	}
	if got.sourceRecords != nil || got.upcomingEvents != nil {
		t.Errorf("empty repository results = (%#v, %#v), want nil slices", got.sourceRecords, got.upcomingEvents)
	}
}

func TestAssembleDailyBriefInputValidatesBeforeRetrieval(t *testing.T) {
	valid := validDailyBriefInputQuery()
	tests := []struct {
		name     string
		query    dailyBriefInputQuery
		contains string
	}{
		{name: "missing region", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.region = "" }), contains: "unsupported region"},
		{name: "unsupported region", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.region = "asia" }), contains: "unsupported region"},
		{name: "missing publication start", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.publicationWindowStart = time.Time{} }), contains: "publication window start is required"},
		{name: "missing publication end", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.publicationWindowEnd = time.Time{} }), contains: "publication window end is required"},
		{name: "reversed publication window", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) {
			query.publicationWindowEnd = query.publicationWindowStart.Add(-time.Nanosecond)
		}), contains: "publication window end must not be before"},
		{name: "zero source limit", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.sourceRecordLimit = 0 }), contains: "source record limit must be between"},
		{name: "negative source limit", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.sourceRecordLimit = -1 }), contains: "source record limit must be between"},
		{name: "source limit above maximum", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) {
			query.sourceRecordLimit = ingestionpostgres.MaxRecentSourceRecordsLimit + 1
		}), contains: "source record limit must be between"},
		{name: "missing event start", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.eventWindowStart = time.Time{} }), contains: "event window start is required"},
		{name: "missing event end", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.eventWindowEnd = time.Time{} }), contains: "event window end is required"},
		{name: "reversed event window", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.eventWindowEnd = query.eventWindowStart.Add(-time.Nanosecond) }), contains: "event window end must not be before"},
		{name: "zero event limit", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.upcomingEventLimit = 0 }), contains: "upcoming event limit must be between"},
		{name: "negative event limit", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) { query.upcomingEventLimit = -1 }), contains: "upcoming event limit must be between"},
		{name: "event limit above maximum", query: withDailyBriefQuery(valid, func(query *dailyBriefInputQuery) {
			query.upcomingEventLimit = calendarpostgres.MaxUpcomingEventsLimit + 1
		}), contains: "upcoming event limit must be between"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := assembleDailyBriefInput(t.Context(), panicDailyBriefSourceRecords{}, panicDailyBriefEvents{}, test.query)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("assembleDailyBriefInput() error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestAssembleDailyBriefInputPreservesSourceRecordFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "repository failure", err: errors.New("source database unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceRepository := &dailyBriefSourceRecordsStub{err: test.err}
			got, err := assembleDailyBriefInput(t.Context(), sourceRepository, panicDailyBriefEvents{}, validDailyBriefInputQuery())
			if err == nil || !errors.Is(err, test.err) || !strings.Contains(err.Error(), "retrieve daily brief source records") {
				t.Fatalf("assembleDailyBriefInput() error = %v, want contextual %v", err, test.err)
			}
			if !reflect.DeepEqual(got, dailyBriefInput{}) {
				t.Errorf("assembleDailyBriefInput() input = %#v, want zero value", got)
			}
		})
	}
}

func TestAssembleDailyBriefInputPreservesUpcomingEventFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "repository failure", err: errors.New("event database unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceRepository := &dailyBriefSourceRecordsStub{records: []ingestionpostgres.StoredSourceRecord{newStoredSourceRecord("record", time.Now())}}
			eventRepository := &dailyBriefEventsStub{err: test.err}
			got, err := assembleDailyBriefInput(t.Context(), sourceRepository, eventRepository, validDailyBriefInputQuery())
			if err == nil || !errors.Is(err, test.err) || !strings.Contains(err.Error(), "retrieve daily brief upcoming events") {
				t.Fatalf("assembleDailyBriefInput() error = %v, want contextual %v", err, test.err)
			}
			if sourceRepository.calls != 1 || eventRepository.calls != 1 {
				t.Errorf("repository calls = (%d, %d), want (1, 1)", sourceRepository.calls, eventRepository.calls)
			}
			if !reflect.DeepEqual(got, dailyBriefInput{}) {
				t.Errorf("assembleDailyBriefInput() input = %#v, want zero value", got)
			}
		})
	}
}

type dailyBriefSourceRecordsStub struct {
	records     []ingestionpostgres.StoredSourceRecord
	err         error
	calls       int
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (repository *dailyBriefSourceRecordsStub) RecentSourceRecords(
	_ context.Context,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]ingestionpostgres.StoredSourceRecord, error) {
	repository.calls++
	repository.windowStart = windowStart
	repository.windowEnd = windowEnd
	repository.limit = limit
	return repository.records, repository.err
}

type dailyBriefEventsStub struct {
	events      []calendarpostgres.StoredEvent
	err         error
	calls       int
	region      calendar.Region
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (repository *dailyBriefEventsStub) UpcomingEvents(
	_ context.Context,
	region calendar.Region,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]calendarpostgres.StoredEvent, error) {
	repository.calls++
	repository.region = region
	repository.windowStart = windowStart
	repository.windowEnd = windowEnd
	repository.limit = limit
	return repository.events, repository.err
}

type panicDailyBriefSourceRecords struct{}

func (panicDailyBriefSourceRecords) RecentSourceRecords(
	context.Context,
	time.Time,
	time.Time,
	int,
) ([]ingestionpostgres.StoredSourceRecord, error) {
	panic("source record retrieval must not run")
}

type panicDailyBriefEvents struct{}

func (panicDailyBriefEvents) UpcomingEvents(
	context.Context,
	calendar.Region,
	time.Time,
	time.Time,
	int,
) ([]calendarpostgres.StoredEvent, error) {
	panic("upcoming event retrieval must not run")
}

func validDailyBriefInputQuery() dailyBriefInputQuery {
	publicationStart := time.Date(2026, time.July, 11, 6, 0, 0, 0, time.UTC)
	eventStart := time.Date(2026, time.July, 12, 6, 0, 0, 0, time.UTC)
	return dailyBriefInputQuery{
		region:                 calendar.RegionEurozone,
		publicationWindowStart: publicationStart,
		publicationWindowEnd:   publicationStart.Add(24 * time.Hour),
		sourceRecordLimit:      20,
		eventWindowStart:       eventStart,
		eventWindowEnd:         eventStart.Add(48 * time.Hour),
		upcomingEventLimit:     10,
	}
}

func withDailyBriefQuery(
	query dailyBriefInputQuery,
	update func(*dailyBriefInputQuery),
) dailyBriefInputQuery {
	update(&query)
	return query
}

func newStoredSourceRecord(sourceItemID string, publishedAt time.Time) ingestionpostgres.StoredSourceRecord {
	return ingestionpostgres.StoredSourceRecord{
		ID: "record-" + sourceItemID,
		SourceRecord: ingestion.SourceRecord{
			Source:       "example-news",
			SourceItemID: sourceItemID,
			OriginalURL:  "https://example.com/news/" + sourceItemID,
			Title:        "Source record " + sourceItemID,
			PublishedAt:  publishedAt,
			RetrievedAt:  publishedAt.Add(time.Minute),
		},
		CreatedAt: publishedAt.Add(2 * time.Minute),
		UpdatedAt: publishedAt.Add(3 * time.Minute),
		CreatedBy: "rss-ingestion",
		UpdatedBy: "rss-ingestion",
	}
}

func newStoredEvent(externalID string, scheduledAt time.Time) calendarpostgres.StoredEvent {
	return calendarpostgres.StoredEvent{
		ID: "event-" + externalID,
		Event: calendar.Event{
			Source:          "official-calendar",
			ExternalEventID: externalID,
			Name:            "Economic event " + externalID,
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     scheduledAt,
			SourceURL:       "https://example.com/calendar/" + externalID,
			RetrievedAt:     scheduledAt.Add(-time.Hour),
		},
		CreatedAt: scheduledAt.Add(-2 * time.Hour),
		UpdatedAt: scheduledAt.Add(-time.Hour),
		CreatedBy: "calendar-ingestion",
		UpdatedBy: "calendar-ingestion",
	}
}
