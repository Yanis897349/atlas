package intelligencecmd

import (
	"context"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

const validEventID = "00000000-0000-0000-0000-000000000085"

func validEventContextQuery() intelligence.EventContextQuery {
	start := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	return intelligence.EventContextQuery{
		EventID:                  validEventID,
		PublicationWindowStart:   start,
		PublicationWindowEnd:     start.Add(4 * time.Hour),
		SourceRecordLimit:        10,
		ObservationLimit:         intelligence.MaxEventObservationsLimit,
		ObservationRevisionLimit: intelligence.MaxEventObservationsLimit,
	}
}

func validEmbeddingBatch() search.EmbeddingBatch {
	return search.EmbeddingBatch{
		Provider: "openai",
		Model:    "embedding-model",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{1, 2},
		}},
	}
}

func storedEventFixture(name string, scheduledAt time.Time) calendar.StoredEvent {
	return calendar.StoredEvent{
		ID: validEventID,
		Event: calendar.Event{
			Source:          "official-calendar",
			ExternalEventID: "event-85",
			Name:            name,
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeInflation,
			ScheduledAt:     scheduledAt,
			SourceURL:       "https://example.com/calendar/event-85",
			RetrievedAt:     scheduledAt.Add(-time.Hour),
		},
		CreatedAt: scheduledAt.Add(-2 * time.Hour),
		UpdatedAt: scheduledAt.Add(-time.Hour),
		CreatedBy: "calendar-ingestion",
		UpdatedBy: "calendar-refresh",
	}
}

func similarSourceRecordFixture(
	id string,
	title string,
	publishedAt time.Time,
	distance float64,
) search.SimilarSourceRecord {
	return search.SimilarSourceRecord{
		SourceRecord: ingestion.StoredSourceRecord{
			ID: id,
			SourceRecord: ingestion.SourceRecord{
				Source:       "publisher",
				SourceItemID: "item-" + id,
				OriginalURL:  "https://example.com/news/" + id,
				Title:        title,
				PublishedAt:  publishedAt,
				RetrievedAt:  publishedAt.Add(time.Minute),
			},
			CreatedAt: publishedAt.Add(2 * time.Minute),
			UpdatedAt: publishedAt.Add(3 * time.Minute),
			CreatedBy: "rss-ingestion",
			UpdatedBy: "rss-refresh",
		},
		Provider:       "openai",
		Model:          "embedding-model",
		CosineDistance: distance,
	}
}

type economicEventReaderStub struct {
	event calendar.StoredEvent
	err   error
	id    string
}

func (reader *economicEventReaderStub) EconomicEvent(_ context.Context, id string) (calendar.StoredEvent, error) {
	reader.id = id
	return reader.event, reader.err
}

type observationReaderStub struct {
	results []intelligence.StoredObservation
	err     error
	eventID string
	limit   int
}

func (reader *observationReaderStub) EventObservations(
	_ context.Context,
	eventID string,
	limit int,
) ([]intelligence.StoredObservation, error) {
	reader.eventID = eventID
	reader.limit = limit
	return reader.results, reader.err
}

func emptyObservationReader() intelligence.ObservationReader {
	return &observationReaderStub{results: []intelligence.StoredObservation{}}
}

func observationValue(value string) *string {
	return &value
}

type embedderStub struct {
	batch  search.EmbeddingBatch
	err    error
	inputs []search.EmbeddingInput
}

func (embedder *embedderStub) Embed(
	_ context.Context,
	inputs []search.EmbeddingInput,
) (search.EmbeddingBatch, error) {
	embedder.inputs = append([]search.EmbeddingInput(nil), inputs...)
	return embedder.batch, embedder.err
}

type similarSourceRecordReaderStub struct {
	results  []search.SimilarSourceRecord
	err      error
	provider string
	model    string
	vector   []float32
	filters  search.SimilarSourceRecordFilters
	limit    int
}

func (reader *similarSourceRecordReaderStub) SimilarSourceRecords(
	_ context.Context,
	provider string,
	model string,
	vector []float32,
	filters search.SimilarSourceRecordFilters,
	limit int,
) ([]search.SimilarSourceRecord, error) {
	reader.provider = provider
	reader.model = model
	reader.vector = append([]float32(nil), vector...)
	reader.filters = filters
	reader.limit = limit
	return reader.results, reader.err
}

type panicEmbedder struct{}

func (panicEmbedder) Embed(context.Context, []search.EmbeddingInput) (search.EmbeddingBatch, error) {
	panic("embedder must not be called")
}

type panicObservationReader struct{}

func (panicObservationReader) EventObservations(
	context.Context,
	string,
	int,
) ([]intelligence.StoredObservation, error) {
	panic("observation reader must not be called")
}

type panicObservationRevisionReader struct{}

func (panicObservationRevisionReader) ObservationRevisions(
	context.Context,
	string,
	string,
	string,
	int,
) ([]intelligence.StoredObservation, error) {
	panic("observation revision reader must not be called")
}

type panicSimilarSourceRecordReader struct{}

func (panicSimilarSourceRecordReader) SimilarSourceRecords(
	context.Context,
	string,
	string,
	[]float32,
	search.SimilarSourceRecordFilters,
	int,
) ([]search.SimilarSourceRecord, error) {
	panic("similar source record reader must not be called")
}
