package intelligence

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5"
)

func TestAssembleEventContextUsesExactEventNameAndPreservesOrderedCanonicalResults(t *testing.T) {
	location := time.FixedZone("CEST", 2*60*60)
	windowStart := time.Date(2026, time.July, 10, 9, 0, 0, 0, location)
	windowEnd := windowStart.Add(6 * time.Hour)
	event := storedEventFixture("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "  Consumer Price Index  ")
	results := []search.SimilarSourceRecord{
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000002", "Closer source", 0.1),
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000001", "Farther source", 0.4),
	}
	events := &economicEventReaderStub{event: event}
	embedder := &embedderStub{batch: search.EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{0.25, 0.5},
		}},
	}}
	sources := &similarSourceRecordReaderStub{results: results}
	query := EventContextQuery{
		EventID:                "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA",
		PublicationWindowStart: windowStart,
		PublicationWindowEnd:   windowEnd,
		SourceRecordLimit:      17,
	}

	got, err := AssembleEventContext(t.Context(), events, embedder, sources, query)
	if err != nil {
		t.Fatalf("AssembleEventContext() error = %v", err)
	}
	want := EventContext{
		Event:                  event,
		PublicationWindowStart: windowStart.UTC(),
		PublicationWindowEnd:   windowEnd.UTC(),
		SourceRecords:          results,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AssembleEventContext() = %#v, want %#v", got, want)
	}
	if events.calls != 1 || events.id != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("economic event retrieval = (%d, %q), want normalized UUID", events.calls, events.id)
	}
	wantInputs := []search.EmbeddingInput{{SourceRecordID: "semantic-search-query", Text: event.Name}}
	if embedder.calls != 1 || !reflect.DeepEqual(embedder.inputs, wantInputs) {
		t.Errorf("embedding call = (%d, %#v), want exact persisted event name", embedder.calls, embedder.inputs)
	}
	if sources.calls != 1 || sources.provider != "openai" || sources.model != "embedding-model" ||
		!reflect.DeepEqual(sources.vector, []float32{0.25, 0.5}) || sources.limit != 17 {
		t.Errorf(
			"source retrieval = (%d, %q, %q, %#v, %d), want normalized provenance, vector, and limit",
			sources.calls,
			sources.provider,
			sources.model,
			sources.vector,
			sources.limit,
		)
	}
	if sources.filters.Source != nil || sources.filters.PublicationWindowStart == nil ||
		sources.filters.PublicationWindowEnd == nil ||
		*sources.filters.PublicationWindowStart != windowStart.UTC() ||
		*sources.filters.PublicationWindowEnd != windowEnd.UTC() {
		t.Errorf("source filters = %#v, want unfiltered source and normalized inclusive window", sources.filters)
	}
}

func TestAssembleEventContextPreservesNonNilEmptySourceResults(t *testing.T) {
	want := []search.SimilarSourceRecord{}
	got, err := AssembleEventContext(
		t.Context(),
		&economicEventReaderStub{event: storedEventFixture(validEventID, "Inflation")},
		&embedderStub{batch: validEmbeddingBatch()},
		&similarSourceRecordReaderStub{results: want},
		validEventContextQuery(),
	)
	if err != nil {
		t.Fatalf("AssembleEventContext() error = %v", err)
	}
	if got.SourceRecords == nil || !reflect.DeepEqual(got.SourceRecords, want) {
		t.Errorf("AssembleEventContext().SourceRecords = %#v, want non-nil empty result", got.SourceRecords)
	}
}

func TestAssembleEventContextRejectsInvalidInputBeforeDependencies(t *testing.T) {
	valid := validEventContextQuery()
	tests := []struct {
		name     string
		query    EventContextQuery
		contains string
	}{
		{name: "missing event ID", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.EventID = "" }), contains: "event ID must be a UUID"},
		{name: "malformed event ID", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.EventID = "not-a-uuid" }), contains: "event ID must be a UUID"},
		{name: "invalid event ID separators", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.EventID = "00000000X0000X0000X0000X000000000083" }), contains: "event ID must be a UUID"},
		{name: "missing publication start", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.PublicationWindowStart = time.Time{} }), contains: "publication window start is required"},
		{name: "missing publication end", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.PublicationWindowEnd = time.Time{} }), contains: "publication window end is required"},
		{name: "reversed publication window", query: withEventContextQuery(valid, func(query *EventContextQuery) {
			query.PublicationWindowEnd = query.PublicationWindowStart.Add(-time.Nanosecond)
		}), contains: "publication window end must not be before start"},
		{name: "zero source record limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.SourceRecordLimit = 0 }), contains: "source record limit must be between"},
		{name: "negative source record limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.SourceRecordLimit = -1 }), contains: "source record limit must be between"},
		{name: "source record limit above maximum", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.SourceRecordLimit = search.MaxSimilarSourceRecordsLimit + 1 }), contains: "source record limit must be between"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AssembleEventContext(
				t.Context(),
				panicEconomicEventReader{},
				panicEmbedder{},
				panicSimilarSourceRecordReader{},
				test.query,
			)
			if err == nil || !strings.Contains(err.Error(), "validate economic event context query") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("AssembleEventContext() error = %v, want validation containing %q", err, test.contains)
			}
			if !reflect.DeepEqual(got, EventContext{}) {
				t.Errorf("AssembleEventContext() = %#v, want zero context", got)
			}
		})
	}
}

func TestAssembleEventContextPreservesDependencyFailures(t *testing.T) {
	tests := []struct {
		name       string
		eventErr   error
		embedErr   error
		sourcesErr error
		contains   string
	}{
		{name: "event repository", eventErr: errors.New("calendar unavailable"), contains: "retrieve economic event"},
		{name: "event not found", eventErr: pgx.ErrNoRows, contains: "retrieve economic event"},
		{name: "event cancellation", eventErr: context.Canceled, contains: "retrieve economic event"},
		{name: "embedding provider", embedErr: errors.New("provider unavailable"), contains: "embed semantic search query with provider"},
		{name: "embedding cancellation", embedErr: context.Canceled, contains: "embed semantic search query with provider"},
		{name: "source repository", sourcesErr: errors.New("search unavailable"), contains: "retrieve similar source records"},
		{name: "source cancellation", sourcesErr: context.Canceled, contains: "retrieve similar source records"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := &economicEventReaderStub{
				event: storedEventFixture(validEventID, "Inflation"),
				err:   test.eventErr,
			}
			embedder := &embedderStub{batch: validEmbeddingBatch(), err: test.embedErr}
			sources := &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}, err: test.sourcesErr}

			got, err := AssembleEventContext(t.Context(), events, embedder, sources, validEventContextQuery())
			wantErr := test.eventErr
			if wantErr == nil {
				wantErr = test.embedErr
			}
			if wantErr == nil {
				wantErr = test.sourcesErr
			}
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), test.contains) ||
				!strings.Contains(err.Error(), failureContext(test.eventErr)) {
				t.Fatalf("AssembleEventContext() error = %v, want contextual %v", err, wantErr)
			}
			if !reflect.DeepEqual(got, EventContext{}) {
				t.Errorf("AssembleEventContext() = %#v, want zero context", got)
			}
			if test.eventErr != nil && (embedder.calls != 0 || sources.calls != 0) {
				t.Errorf("downstream calls after event failure = (%d, %d), want none", embedder.calls, sources.calls)
			}
			if test.embedErr != nil && sources.calls != 0 {
				t.Errorf("source calls after embedding failure = %d, want none", sources.calls)
			}
		})
	}
}

func TestAssembleEventContextPreservesMalformedEmbeddingFailure(t *testing.T) {
	got, err := AssembleEventContext(
		t.Context(),
		&economicEventReaderStub{event: storedEventFixture(validEventID, "Inflation")},
		&embedderStub{batch: search.EmbeddingBatch{Provider: "provider", Model: "model"}},
		panicSimilarSourceRecordReader{},
		validEventContextQuery(),
	)
	if err == nil || !strings.Contains(err.Error(), "retrieve economic event source records") ||
		!strings.Contains(err.Error(), "validate semantic search query embedding") {
		t.Fatalf("AssembleEventContext() error = %v, want contextual malformed embedding failure", err)
	}
	if !reflect.DeepEqual(got, EventContext{}) {
		t.Errorf("AssembleEventContext() = %#v, want zero context", got)
	}
}

const validEventID = "00000000-0000-0000-0000-000000000083"

func validEventContextQuery() EventContextQuery {
	windowStart := time.Date(2026, time.July, 10, 8, 0, 0, 0, time.UTC)
	return EventContextQuery{
		EventID:                validEventID,
		PublicationWindowStart: windowStart,
		PublicationWindowEnd:   windowStart.Add(24 * time.Hour),
		SourceRecordLimit:      20,
	}
}

func withEventContextQuery(query EventContextQuery, update func(*EventContextQuery)) EventContextQuery {
	update(&query)
	return query
}

func storedEventFixture(id, name string) calendar.StoredEvent {
	scheduledAt := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	return calendar.StoredEvent{
		ID: id,
		Event: calendar.Event{
			Source:          "official-calendar",
			ExternalEventID: "event-83",
			Name:            name,
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeInflation,
			ScheduledAt:     scheduledAt,
			SourceURL:       "https://example.com/calendar/event-83",
			RetrievedAt:     scheduledAt.Add(-time.Hour),
		},
		CreatedAt: scheduledAt.Add(-2 * time.Hour),
		UpdatedAt: scheduledAt.Add(-time.Hour),
		CreatedBy: "calendar-ingestion",
		UpdatedBy: "calendar-refresh",
	}
}

func similarSourceRecordFixture(id, title string, distance float64) search.SimilarSourceRecord {
	publishedAt := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)
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

func validEmbeddingBatch() search.EmbeddingBatch {
	return search.EmbeddingBatch{
		Provider: "provider",
		Model:    "model",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{1, 2},
		}},
	}
}

func failureContext(eventErr error) string {
	if eventErr != nil {
		return "retrieve economic event"
	}
	return "retrieve economic event source records"
}

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
