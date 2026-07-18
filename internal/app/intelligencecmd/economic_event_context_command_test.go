package intelligencecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

func TestRunEconomicEventContextWritesCompleteOrderedContext(t *testing.T) {
	paris := time.FixedZone("Paris", 2*60*60)
	windowStart := time.Date(2026, time.July, 12, 10, 0, 0, 0, paris)
	windowEnd := windowStart.Add(4 * time.Hour)
	event := storedEventFixture("  Consumer Price Index  ", windowEnd)
	events := &economicEventReaderStub{event: event}
	observationResults := []intelligence.StoredObservation{
		{
			ID: "00000000-0000-0000-0000-000000000087",
			Observation: intelligence.Observation{
				EconomicEventID:     event.ID,
				Source:              "official-statistics",
				SourceObservationID: "cpi-2026-07",
				SourceURL:           "https://example.com/releases/cpi-2026-07",
				ObservedAt:          windowEnd.Add(time.Hour),
				Consensus:           observationValue("3.1%"),
				Previous:            observationValue("3.0%"),
				Actual:              observationValue("3.3%"),
			},
			CreatedAt: windowEnd.Add(2 * time.Hour),
			UpdatedAt: windowEnd.Add(3 * time.Hour),
			CreatedBy: "observation-ingestion",
			UpdatedBy: "observation-refresh",
		},
		{
			ID: "00000000-0000-0000-0000-000000000086",
			Observation: intelligence.Observation{
				EconomicEventID:     event.ID,
				Source:              "secondary-statistics",
				SourceObservationID: "cpi-secondary-2026-07",
				SourceURL:           "https://example.org/releases/cpi-2026-07",
				ObservedAt:          windowStart,
				Previous:            observationValue("3.2%"),
			},
			CreatedAt: windowStart.Add(time.Hour),
			UpdatedAt: windowStart.Add(2 * time.Hour),
			CreatedBy: "secondary-ingestion",
			UpdatedBy: "secondary-refresh",
		},
	}
	observations := &observationReaderStub{results: observationResults}
	olderRevision := observationResults[0]
	olderRevision.ID = "00000000-0000-0000-0000-000000000085"
	olderRevision.SourceURL = "https://example.com/releases/cpi-2026-07-initial"
	olderRevision.ObservedAt = windowEnd
	olderRevision.Actual = nil
	observationRevisions := &observationRevisionReaderStub{resultsByCall: [][]intelligence.StoredObservation{
		{observationResults[0], olderRevision},
		{},
	}}
	embedder := &embedderStub{batch: search.EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{1, 2},
		}},
	}}
	results := []search.SimilarSourceRecord{
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000002", "Second", windowStart, 0.1),
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000001", "First", windowStart.Add(time.Hour), 0.4),
	}
	sources := &similarSourceRecordReaderStub{results: results}
	stdout := &bytes.Buffer{}
	query := intelligence.EventContextQuery{
		EventID:                  strings.ToUpper(validEventID),
		PublicationWindowStart:   windowStart,
		PublicationWindowEnd:     windowEnd,
		SourceRecordLimit:        2,
		ObservationLimit:         7,
		ObservationRevisionLimit: 5,
	}

	if err := runEconomicEventContext(
		t.Context(), events, observations, observationRevisions, embedder, sources, stdout, query,
	); err != nil {
		t.Fatalf("runEconomicEventContext() error = %v", err)
	}
	wantRevisionCalls := []observationRevisionReaderInput{
		{eventID: validEventID, source: "official-statistics", sourceObservationID: "cpi-2026-07", limit: 5},
		{eventID: validEventID, source: "secondary-statistics", sourceObservationID: "cpi-secondary-2026-07", limit: 5},
	}
	if !reflect.DeepEqual(observationRevisions.calls, wantRevisionCalls) {
		t.Errorf("observation revision calls = %#v, want %#v", observationRevisions.calls, wantRevisionCalls)
	}
	if events.id != validEventID || observations.eventID != validEventID || observations.limit != 7 ||
		!reflect.DeepEqual(embedder.inputs, []search.EmbeddingInput{{
			SourceRecordID: "semantic-search-query", Text: event.Name,
		}}) || sources.provider != "openai" || sources.model != "embedding-model" ||
		!reflect.DeepEqual(sources.vector, []float32{1, 2}) || sources.limit != 2 ||
		sources.filters.PublicationWindowStart == nil ||
		*sources.filters.PublicationWindowStart != windowStart.UTC() ||
		sources.filters.PublicationWindowEnd == nil || *sources.filters.PublicationWindowEnd != windowEnd.UTC() {
		t.Errorf("orchestration = events %#v, embedder %#v, sources %#v", events, embedder, sources)
	}

	var got economicEventContextOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &encoded); err != nil {
		t.Fatalf("decode output keys: %v", err)
	}
	if _, exists := encoded["observations"]; !exists || len(encoded) != 5 {
		t.Errorf("output keys = %v, want event context schema with observations", reflect.ValueOf(encoded).MapKeys())
	}
	if got.Event.ID != validEventID || got.Event.SourceURL == "" || got.Event.CreatedBy != "calendar-ingestion" ||
		got.Event.UpdatedBy != "calendar-refresh" || got.Event.ScheduledAt != "2026-07-12T12:00:00Z" ||
		got.PublicationWindowStart != "2026-07-12T08:00:00Z" ||
		got.PublicationWindowEnd != "2026-07-12T12:00:00Z" || len(got.Observations) != 2 ||
		got.Observations[0].ID != observationResults[0].ID ||
		got.Observations[1].ID != observationResults[1].ID ||
		got.Observations[0].EconomicEventID != event.ID ||
		got.Observations[0].Source != "official-statistics" ||
		got.Observations[0].SourceObservationID != "cpi-2026-07" ||
		got.Observations[0].SourceURL != "https://example.com/releases/cpi-2026-07" ||
		got.Observations[0].ObservedAt != "2026-07-12T13:00:00Z" ||
		got.Observations[0].Consensus == nil || *got.Observations[0].Consensus != "3.1%" ||
		got.Observations[0].Previous == nil || *got.Observations[0].Previous != "3.0%" ||
		got.Observations[0].Actual == nil || *got.Observations[0].Actual != "3.3%" ||
		got.Observations[0].CreatedAt != "2026-07-12T14:00:00Z" ||
		got.Observations[0].UpdatedAt != "2026-07-12T15:00:00Z" ||
		got.Observations[0].CreatedBy != "observation-ingestion" ||
		got.Observations[0].UpdatedBy != "observation-refresh" ||
		got.Observations[1].Consensus != nil || got.Observations[1].Actual != nil ||
		got.Observations[1].Previous == nil || *got.Observations[1].Previous != "3.2%" ||
		len(got.Observations[0].Revisions) != 2 ||
		got.Observations[0].Revisions[0].ID != observationResults[0].ID ||
		got.Observations[0].Revisions[1].ID != olderRevision.ID ||
		got.Observations[0].Revisions[1].SourceURL != olderRevision.SourceURL ||
		got.Observations[0].Revisions[1].Actual != nil ||
		got.Observations[0].Revisions[1].ObservedAt != "2026-07-12T12:00:00Z" ||
		got.Observations[1].Revisions == nil || len(got.Observations[1].Revisions) != 0 ||
		len(got.SourceRecords) != 2 ||
		got.SourceRecords[0].ID != results[0].SourceRecord.ID ||
		got.SourceRecords[1].ID != results[1].SourceRecord.ID ||
		got.SourceRecords[0].PublishedAt != "2026-07-12T08:00:00Z" ||
		got.SourceRecords[0].CreatedBy != "rss-ingestion" || got.SourceRecords[0].UpdatedBy != "rss-refresh" ||
		got.SourceRecords[0].Provider != "openai" || got.SourceRecords[0].Model != "embedding-model" ||
		got.SourceRecords[0].CosineDistance != 0.1 {
		t.Errorf("output = %#v, want complete UTC event context in repository order", got)
	}
	if !strings.Contains(stdout.String(), `"consensus":null`) ||
		!strings.Contains(stdout.String(), `"actual":null`) {
		t.Errorf("output = %q, want nullable observation values encoded as null", stdout.String())
	}
}

func TestRunEconomicEventContextWritesEmptyArrays(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runEconomicEventContext(
		t.Context(),
		&economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())},
		&observationReaderStub{results: []intelligence.StoredObservation{}},
		&observationRevisionReaderStub{},
		&embedderStub{batch: validEmbeddingBatch()},
		&similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}},
		stdout,
		validEventContextQuery(),
	)
	if err != nil {
		t.Fatalf("runEconomicEventContext() error = %v", err)
	}
	var output struct {
		Observations  []economicEventContextObservationOutput `json:"observations"`
		SourceRecords []economicEventSourceOutput             `json:"source_records"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output.Observations == nil || len(output.Observations) != 0 ||
		output.SourceRecords == nil || len(output.SourceRecords) != 0 ||
		!strings.Contains(stdout.String(), `"observations":[]`) ||
		!strings.Contains(stdout.String(), `"source_records":[]`) {
		t.Errorf("arrays = (%#v, %#v) (%q), want non-nil empty JSON arrays", output.Observations, output.SourceRecords, stdout.String())
	}
}

func TestRunEconomicEventContextPreservesFailuresWithoutBufferedOutput(t *testing.T) {
	wantErr := errors.New("dependency unavailable")
	tests := []struct {
		name         string
		events       intelligence.EconomicEventReader
		observations intelligence.ObservationReader
		revisions    intelligence.ObservationRevisionReader
		embedder     search.Embedder
		sources      search.SimilarSourceRecordReader
		stdout       io.Writer
		contains     string
		wantErr      error
	}{
		{name: "event repository", events: &economicEventReaderStub{err: wantErr}, observations: panicObservationReader{}, revisions: panicObservationRevisionReader{}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event", wantErr: wantErr},
		{name: "cancellation", events: &economicEventReaderStub{err: context.Canceled}, observations: panicObservationReader{}, revisions: panicObservationRevisionReader{}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event", wantErr: context.Canceled},
		{name: "observation repository", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: &observationReaderStub{err: wantErr}, revisions: panicObservationRevisionReader{}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event observations", wantErr: wantErr},
		{name: "observation revision repository", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: &observationReaderStub{results: []intelligence.StoredObservation{{Observation: intelligence.Observation{EconomicEventID: validEventID, Source: "source", SourceObservationID: "identity"}}}}, revisions: &observationRevisionReaderStub{err: wantErr}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event observation revisions", wantErr: wantErr},
		{name: "provider", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{err: wantErr}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "embed semantic search query", wantErr: wantErr},
		{name: "source repository", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{err: wantErr}, stdout: &bytes.Buffer{}, contains: "retrieve similar source records", wantErr: wantErr},
		{name: "encoding", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{{CosineDistance: math.NaN()}}}, stdout: &bytes.Buffer{}, contains: "encode economic event context"},
		{name: "writer", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}}, stdout: errorWriter{err: wantErr}, contains: "write economic event context", wantErr: wantErr},
		{name: "short writer", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}}, stdout: shortWriter{}, contains: "short write", wantErr: io.ErrShortWrite},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runEconomicEventContext(
				t.Context(),
				test.events,
				test.observations,
				test.revisions,
				test.embedder,
				test.sources,
				test.stdout,
				validEventContextQuery(),
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				(test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Fatalf("error = %v, want contextual failure containing %q", err, test.contains)
			}
			if buffer, ok := test.stdout.(*bytes.Buffer); ok && buffer.Len() != 0 {
				t.Errorf("stdout = %q, want no JSON", buffer.String())
			}
		})
	}
}

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

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}
