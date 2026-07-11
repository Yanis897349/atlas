package dailybrief

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestAssembleInputPreservesRepositoryResults(t *testing.T) {
	publicationStart := time.Date(2026, time.July, 11, 7, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	publicationEnd := publicationStart.Add(4 * time.Hour)
	eventStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	eventEnd := eventStart.Add(24 * time.Hour)
	records := []ingestion.StoredSourceRecord{newStoredSourceRecord("newer", publicationEnd), newStoredSourceRecord("older", publicationStart)}
	events := []calendar.StoredEvent{newStoredEvent("first", eventStart), newStoredEvent("second", eventEnd)}
	sourceRepository := &sourceRecordsStub{records: records}
	eventRepository := &eventsStub{events: events}
	query := InputQuery{
		Region:                 calendar.RegionUnitedStates,
		PublicationWindowStart: publicationStart,
		PublicationWindowEnd:   publicationEnd,
		SourceRecordLimit:      12,
		EventWindowStart:       eventStart,
		EventWindowEnd:         eventEnd,
		UpcomingEventLimit:     8,
	}

	got, err := AssembleInput(t.Context(), sourceRepository, eventRepository, query)
	if err != nil {
		t.Fatalf("AssembleInput() error = %v", err)
	}
	want := Input{
		Region:                 calendar.RegionUnitedStates,
		PublicationWindowStart: publicationStart.UTC(),
		PublicationWindowEnd:   publicationEnd.UTC(),
		EventWindowStart:       eventStart.UTC(),
		EventWindowEnd:         eventEnd.UTC(),
		SourceRecords:          records,
		UpcomingEvents:         events,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AssembleInput() = %#v, want %#v", got, want)
	}
	if sourceRepository.calls != 1 || sourceRepository.windowStart != publicationStart.UTC() ||
		sourceRepository.windowEnd != publicationEnd.UTC() || sourceRepository.limit != 12 {
		t.Errorf("source repository call = (%d, %v, %v, %d)", sourceRepository.calls, sourceRepository.windowStart, sourceRepository.windowEnd, sourceRepository.limit)
	}
	if eventRepository.calls != 1 || eventRepository.region != calendar.RegionUnitedStates ||
		eventRepository.windowStart != eventStart.UTC() || eventRepository.windowEnd != eventEnd.UTC() || eventRepository.limit != 8 {
		t.Errorf("event repository call = (%d, %q, %v, %v, %d)", eventRepository.calls, eventRepository.region, eventRepository.windowStart, eventRepository.windowEnd, eventRepository.limit)
	}
}

func TestAssembleInputAcceptsEmptyResultsAndEqualWindows(t *testing.T) {
	query := validInputQuery()
	query.PublicationWindowEnd = query.PublicationWindowStart
	query.EventWindowEnd = query.EventWindowStart

	got, err := AssembleInput(t.Context(), &sourceRecordsStub{}, &eventsStub{}, query)
	if err != nil {
		t.Fatalf("AssembleInput() error = %v", err)
	}
	if got.SourceRecords != nil || got.UpcomingEvents != nil {
		t.Errorf("empty repository results = (%#v, %#v), want nil slices", got.SourceRecords, got.UpcomingEvents)
	}
}

func TestAssembleInputValidatesBeforeRetrieval(t *testing.T) {
	valid := validInputQuery()
	tests := []struct {
		name     string
		query    InputQuery
		contains string
	}{
		{name: "missing region", query: withInputQuery(valid, func(query *InputQuery) { query.Region = "" }), contains: "unsupported region"},
		{name: "unsupported region", query: withInputQuery(valid, func(query *InputQuery) { query.Region = "asia" }), contains: "unsupported region"},
		{name: "missing publication start", query: withInputQuery(valid, func(query *InputQuery) { query.PublicationWindowStart = time.Time{} }), contains: "publication window start is required"},
		{name: "missing publication end", query: withInputQuery(valid, func(query *InputQuery) { query.PublicationWindowEnd = time.Time{} }), contains: "publication window end is required"},
		{name: "reversed publication window", query: withInputQuery(valid, func(query *InputQuery) {
			query.PublicationWindowEnd = query.PublicationWindowStart.Add(-time.Nanosecond)
		}), contains: "publication window end must not be before"},
		{name: "zero source limit", query: withInputQuery(valid, func(query *InputQuery) { query.SourceRecordLimit = 0 }), contains: "source record limit must be between"},
		{name: "source limit above maximum", query: withInputQuery(valid, func(query *InputQuery) { query.SourceRecordLimit = ingestion.MaxRecentSourceRecordsLimit + 1 }), contains: "source record limit must be between"},
		{name: "missing event start", query: withInputQuery(valid, func(query *InputQuery) { query.EventWindowStart = time.Time{} }), contains: "event window start is required"},
		{name: "missing event end", query: withInputQuery(valid, func(query *InputQuery) { query.EventWindowEnd = time.Time{} }), contains: "event window end is required"},
		{name: "reversed event window", query: withInputQuery(valid, func(query *InputQuery) { query.EventWindowEnd = query.EventWindowStart.Add(-time.Nanosecond) }), contains: "event window end must not be before"},
		{name: "zero event limit", query: withInputQuery(valid, func(query *InputQuery) { query.UpcomingEventLimit = 0 }), contains: "upcoming event limit must be between"},
		{name: "event limit above maximum", query: withInputQuery(valid, func(query *InputQuery) { query.UpcomingEventLimit = calendar.MaxUpcomingEventsLimit + 1 }), contains: "upcoming event limit must be between"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := AssembleInput(t.Context(), panicSourceRecords{}, panicEvents{}, test.query)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("AssembleInput() error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestAssembleInputPreservesRepositoryFailures(t *testing.T) {
	t.Run("source records", func(t *testing.T) {
		wantErr := errors.New("source database unavailable")
		got, err := AssembleInput(t.Context(), &sourceRecordsStub{err: wantErr}, panicEvents{}, validInputQuery())
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "retrieve daily brief source records") || !reflect.DeepEqual(got, Input{}) {
			t.Fatalf("AssembleInput() = (%#v, %v), want contextual source failure", got, err)
		}
	})
	t.Run("upcoming events", func(t *testing.T) {
		wantErr := context.Canceled
		sources := &sourceRecordsStub{records: []ingestion.StoredSourceRecord{newStoredSourceRecord("record", time.Now())}}
		events := &eventsStub{err: wantErr}
		got, err := AssembleInput(t.Context(), sources, events, validInputQuery())
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "retrieve daily brief upcoming events") || !reflect.DeepEqual(got, Input{}) {
			t.Fatalf("AssembleInput() = (%#v, %v), want contextual event failure", got, err)
		}
	})
}

type sourceRecordsStub struct {
	records     []ingestion.StoredSourceRecord
	err         error
	calls       int
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (repository *sourceRecordsStub) RecentSourceRecords(_ context.Context, windowStart, windowEnd time.Time, limit int) ([]ingestion.StoredSourceRecord, error) {
	repository.calls++
	repository.windowStart, repository.windowEnd, repository.limit = windowStart, windowEnd, limit
	return repository.records, repository.err
}

type eventsStub struct {
	events      []calendar.StoredEvent
	err         error
	calls       int
	region      calendar.Region
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (repository *eventsStub) UpcomingEvents(_ context.Context, region calendar.Region, windowStart, windowEnd time.Time, limit int) ([]calendar.StoredEvent, error) {
	repository.calls++
	repository.region, repository.windowStart, repository.windowEnd, repository.limit = region, windowStart, windowEnd, limit
	return repository.events, repository.err
}

type panicSourceRecords struct{}

func (panicSourceRecords) RecentSourceRecords(context.Context, time.Time, time.Time, int) ([]ingestion.StoredSourceRecord, error) {
	panic("source record retrieval must not run")
}

type panicEvents struct{}

func (panicEvents) UpcomingEvents(context.Context, calendar.Region, time.Time, time.Time, int) ([]calendar.StoredEvent, error) {
	panic("upcoming event retrieval must not run")
}

func validInputQuery() InputQuery {
	publicationStart := time.Date(2026, time.July, 11, 6, 0, 0, 0, time.UTC)
	eventStart := publicationStart.Add(24 * time.Hour)
	return InputQuery{
		Region: calendar.RegionEurozone, PublicationWindowStart: publicationStart,
		PublicationWindowEnd: publicationStart.Add(24 * time.Hour), SourceRecordLimit: 20,
		EventWindowStart: eventStart, EventWindowEnd: eventStart.Add(48 * time.Hour), UpcomingEventLimit: 10,
	}
}

func withInputQuery(query InputQuery, update func(*InputQuery)) InputQuery {
	update(&query)
	return query
}

func newStoredSourceRecord(sourceItemID string, publishedAt time.Time) ingestion.StoredSourceRecord {
	return ingestion.StoredSourceRecord{ID: "record-" + sourceItemID, SourceRecord: ingestion.SourceRecord{
		Source: "example-news", SourceItemID: sourceItemID, OriginalURL: "https://example.com/news/" + sourceItemID,
		Title: "Source record " + sourceItemID, PublishedAt: publishedAt, RetrievedAt: publishedAt.Add(time.Minute),
	}, CreatedAt: publishedAt.Add(2 * time.Minute), UpdatedAt: publishedAt.Add(3 * time.Minute), CreatedBy: "rss-ingestion", UpdatedBy: "rss-ingestion"}
}

func newStoredEvent(externalID string, scheduledAt time.Time) calendar.StoredEvent {
	return calendar.StoredEvent{ID: "event-" + externalID, Event: calendar.Event{
		Source: "official-calendar", ExternalEventID: externalID, Name: "Economic event " + externalID,
		Region: calendar.RegionUnitedStates, Type: calendar.EventTypeGDP, ScheduledAt: scheduledAt,
		SourceURL: "https://example.com/calendar/" + externalID, RetrievedAt: scheduledAt.Add(-time.Hour),
	}, CreatedAt: scheduledAt.Add(-2 * time.Hour), UpdatedAt: scheduledAt.Add(-time.Hour), CreatedBy: "calendar-ingestion", UpdatedBy: "calendar-ingestion"}
}
