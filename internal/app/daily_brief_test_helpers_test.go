package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

const defaultOpenAIResponsesEndpoint = "https://api.openai.com/v1/responses"

type openAIDailyBriefRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIDailyBriefInput struct {
	SourceRecords []struct {
		ID           string `json:"id"`
		SourceItemID string `json:"source_item_id"`
	} `json:"source_records"`
	UpcomingEvents []struct {
		ID              string `json:"id"`
		ExternalEventID string `json:"external_event_id"`
	} `json:"upcoming_events"`
}

type dailyBriefSourceRecordsStub struct {
	records []ingestion.StoredSourceRecord
	err     error
}

func (repository *dailyBriefSourceRecordsStub) RecentSourceRecords(context.Context, time.Time, time.Time, int) ([]ingestion.StoredSourceRecord, error) {
	return repository.records, repository.err
}

type dailyBriefEventsStub struct {
	events []calendar.StoredEvent
	err    error
}

func (repository *dailyBriefEventsStub) UpcomingEvents(context.Context, calendar.Region, time.Time, time.Time, int) ([]calendar.StoredEvent, error) {
	return repository.events, repository.err
}

type panicDailyBriefEvents struct{}

func (panicDailyBriefEvents) UpcomingEvents(context.Context, calendar.Region, time.Time, time.Time, int) ([]calendar.StoredEvent, error) {
	panic("upcoming event retrieval must not run")
}

func validDailyBriefInputQuery() dailybrief.InputQuery {
	publicationStart := time.Date(2026, time.July, 11, 6, 0, 0, 0, time.UTC)
	eventStart := publicationStart.Add(24 * time.Hour)
	return dailybrief.InputQuery{
		Region:                 calendar.RegionEurozone,
		PublicationWindowStart: publicationStart,
		PublicationWindowEnd:   publicationStart.Add(24 * time.Hour),
		SourceRecordLimit:      20,
		EventWindowStart:       eventStart,
		EventWindowEnd:         eventStart.Add(48 * time.Hour),
		UpcomingEventLimit:     10,
	}
}

func dailyBriefGenerationInput() dailybrief.Input {
	query := validDailyBriefInputQuery()
	return dailybrief.Input{
		Region:                 query.Region,
		PublicationWindowStart: query.PublicationWindowStart,
		PublicationWindowEnd:   query.PublicationWindowEnd,
		EventWindowStart:       query.EventWindowStart,
		EventWindowEnd:         query.EventWindowEnd,
		SourceRecords:          []ingestion.StoredSourceRecord{newStoredSourceRecord("news", query.PublicationWindowStart)},
		UpcomingEvents:         []calendar.StoredEvent{newStoredEvent("gdp", query.EventWindowStart)},
	}
}

func newStoredSourceRecord(sourceItemID string, publishedAt time.Time) ingestion.StoredSourceRecord {
	return ingestion.StoredSourceRecord{ID: "record-" + sourceItemID, SourceRecord: ingestion.SourceRecord{
		Source: "example-news", SourceItemID: sourceItemID, OriginalURL: "https://example.com/news/" + sourceItemID,
		Title: "Source record " + sourceItemID, PublishedAt: publishedAt, RetrievedAt: publishedAt.Add(time.Minute),
	}}
}

func newStoredEvent(externalID string, scheduledAt time.Time) calendar.StoredEvent {
	return calendar.StoredEvent{ID: "event-" + externalID, Event: calendar.Event{
		Source: "official-calendar", ExternalEventID: externalID, Name: "Economic event " + externalID,
		Region: calendar.RegionUnitedStates, Type: calendar.EventTypeGDP, ScheduledAt: scheduledAt,
		SourceURL: "https://example.com/calendar/" + externalID, RetrievedAt: scheduledAt.Add(-time.Hour),
	}}
}

type dailyBriefGeneratorStub struct {
	draft dailybrief.Draft
	err   error
	calls int
	input dailybrief.Input
}

func (generator *dailyBriefGeneratorStub) Generate(_ context.Context, input dailybrief.Input) (dailybrief.Generation, error) {
	generator.calls++
	generator.input = input
	return dailybrief.Generation{Provider: "openai", Model: "test-model", Draft: generator.draft}, generator.err
}

func completedOpenAIResponse(outputText string) string {
	encodedText, err := json.Marshal(outputText)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{"status":"completed","output":[{"type":"reasoning"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":%s}]}]}`, encodedText)
}

func writeOpenAIResponse(t *testing.T, response http.ResponseWriter, status int, body string) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if _, err := response.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func validUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil && id.Valid
}

func persistedDailyBriefFixture(sourceID, eventID string) dailybrief.Brief {
	return dailybrief.Brief{
		Region:                 calendar.RegionUnitedStates,
		PublicationWindowStart: time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC),
		PublicationWindowEnd:   time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC),
		EventWindowStart:       time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC),
		EventWindowEnd:         time.Date(2026, time.July, 13, 8, 0, 0, 0, time.UTC),
		Provider:               "openai", Model: "test-model",
		Sections: []dailybrief.Section{
			{Heading: "What matters", Content: "A cited development.", Citations: []dailybrief.Citation{
				{Kind: dailybrief.CitationUpcomingEvent, ID: eventID, Source: "official-calendar", URL: "https://example.com/calendar/brief-event"},
				{Kind: dailybrief.CitationSourceRecord, ID: sourceID, Source: "example-news", URL: "https://example.com/news/brief-source"},
			}},
			{Heading: "What to watch", Content: "The next event.", Citations: []dailybrief.Citation{
				{Kind: dailybrief.CitationUpcomingEvent, ID: eventID, Source: "official-calendar", URL: "https://example.com/calendar/brief-event"},
			}},
		},
	}
}

func persistDailyBriefReferences(t *testing.T, database postgrestest.Database) (string, string) {
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
