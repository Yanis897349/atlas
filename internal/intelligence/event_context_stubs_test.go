package intelligence

import (
	"context"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/search"
)

type economicEventReaderStub struct {
	event calendar.StoredEvent
	err   error
	calls int
	id    string
}

func (reader *economicEventReaderStub) EconomicEvent(_ context.Context, id string) (calendar.StoredEvent, error) {
	reader.calls++
	reader.id = id
	return reader.event, reader.err
}

type observationReaderStub struct {
	results []StoredObservation
	err     error
	calls   int
	eventID string
	limit   int
}

func (reader *observationReaderStub) EventObservations(
	_ context.Context,
	eventID string,
	limit int,
) ([]StoredObservation, error) {
	reader.calls++
	reader.eventID = eventID
	reader.limit = limit
	return reader.results, reader.err
}

type observationRevisionReaderCall struct {
	eventID             string
	source              string
	sourceObservationID string
	limit               int
}

type observationRevisionReaderStub struct {
	results [][]StoredObservation
	err     error
	calls   []observationRevisionReaderCall
}

func (reader *observationRevisionReaderStub) ObservationRevisions(
	_ context.Context,
	eventID string,
	source string,
	sourceObservationID string,
	limit int,
) ([]StoredObservation, error) {
	reader.calls = append(reader.calls, observationRevisionReaderCall{
		eventID: eventID, source: source, sourceObservationID: sourceObservationID, limit: limit,
	})
	if reader.err != nil {
		return nil, reader.err
	}
	index := len(reader.calls) - 1
	if index >= len(reader.results) {
		return []StoredObservation{}, nil
	}
	return reader.results[index], nil
}

type embedderStub struct {
	batch  search.EmbeddingBatch
	err    error
	calls  int
	inputs []search.EmbeddingInput
}

func (embedder *embedderStub) Embed(_ context.Context, inputs []search.EmbeddingInput) (search.EmbeddingBatch, error) {
	embedder.calls++
	embedder.inputs = append([]search.EmbeddingInput(nil), inputs...)
	return embedder.batch, embedder.err
}

type similarSourceRecordReaderStub struct {
	results  []search.SimilarSourceRecord
	err      error
	calls    int
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
	reader.calls++
	reader.provider = provider
	reader.model = model
	reader.vector = append([]float32(nil), vector...)
	reader.filters = filters
	reader.limit = limit
	return reader.results, reader.err
}

type panicEconomicEventReader struct{}

func (panicEconomicEventReader) EconomicEvent(context.Context, string) (calendar.StoredEvent, error) {
	panic("economic event retrieval must not run")
}

type panicObservationReader struct{}

func (panicObservationReader) EventObservations(context.Context, string, int) ([]StoredObservation, error) {
	panic("observation retrieval must not run")
}

type panicObservationRevisionReader struct{}

func (panicObservationRevisionReader) ObservationRevisions(
	context.Context,
	string,
	string,
	string,
	int,
) ([]StoredObservation, error) {
	panic("observation revision retrieval must not run")
}

type panicEmbedder struct{}

func (panicEmbedder) Embed(context.Context, []search.EmbeddingInput) (search.EmbeddingBatch, error) {
	panic("embedding provider must not run")
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
	panic("source record retrieval must not run")
}
