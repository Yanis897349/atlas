package search

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestSearchSourceRecordsEmbedsExactQueryAndPreservesRepositoryResults(t *testing.T) {
	query := "  central bank policy outlook  "
	embedder := &embedderStub{batch: EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []ProviderEmbedding{{
			SourceRecordID: semanticSearchQueryID,
			Vector:         []float32{0.25, 0.5, 0.75},
		}},
	}}
	want := []SimilarSourceRecord{
		semanticSearchResultFixture("00000000-0000-0000-0000-000000000002", "Second canonical headline", 0.1),
		semanticSearchResultFixture("00000000-0000-0000-0000-000000000001", "First canonical headline", 0.3),
	}
	reader := &similarSourceRecordReaderStub{results: want}
	source := "  publisher  "
	paris := time.FixedZone("Paris", 2*60*60)
	windowStart := time.Date(2026, time.July, 12, 10, 0, 0, 0, paris)
	windowEnd := windowStart.Add(6 * time.Hour)
	filters := SimilarSourceRecordFilters{
		Source:                 &source,
		PublicationWindowStart: &windowStart,
		PublicationWindowEnd:   &windowEnd,
	}

	got, err := SearchSourceRecords(t.Context(), embedder, reader, query, filters, 17)
	if err != nil {
		t.Fatalf("SearchSourceRecords() error = %v", err)
	}
	wantInputs := []EmbeddingInput{{SourceRecordID: semanticSearchQueryID, Text: query}}
	if embedder.calls != 1 || !reflect.DeepEqual(embedder.inputs, wantInputs) {
		t.Errorf("embedder call = (%d, %#v), want (1, %#v)", embedder.calls, embedder.inputs, wantInputs)
	}
	if reader.calls != 1 || reader.provider != "openai" || reader.model != "embedding-model" ||
		reader.filters.Source == nil || *reader.filters.Source != "publisher" ||
		reader.filters.PublicationWindowStart == nil ||
		*reader.filters.PublicationWindowStart != windowStart.UTC() ||
		reader.filters.PublicationWindowEnd == nil ||
		*reader.filters.PublicationWindowEnd != windowEnd.UTC() || reader.limit != 17 ||
		!reflect.DeepEqual(reader.queryVector, []float32{0.25, 0.5, 0.75}) {
		t.Errorf(
			"reader call = (%d, %q, %q, %#v, %#v, %d), want normalized filters",
			reader.calls,
			reader.provider,
			reader.model,
			reader.queryVector,
			reader.filters,
			reader.limit,
		)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SearchSourceRecords() = %#v, want %#v", got, want)
	}
	if source != "  publisher  " || windowStart.Location() != paris || windowEnd.Location() != paris {
		t.Errorf("caller filters mutated = (%q, %v, %v)", source, windowStart, windowEnd)
	}

	embedder.batch.Embeddings[0].Vector[0] = 99
	if reader.queryVector[0] != 0.25 {
		t.Errorf("reader query vector changed with provider result: %#v", reader.queryVector)
	}
}

func TestSearchSourceRecordsRejectsInvalidInputBeforeDependencies(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		filters  SimilarSourceRecordFilters
		limit    int
		contains string
	}{
		{name: "blank query", query: " \t\n", limit: 1, contains: "query is required"},
		{name: "blank source", query: "inflation", filters: SimilarSourceRecordFilters{Source: semanticSource(" \t\n")}, limit: 1, contains: "source is required when supplied"},
		{name: "missing window end", query: "inflation", filters: SimilarSourceRecordFilters{PublicationWindowStart: semanticTime(time.Now())}, limit: 1, contains: "start and end must be supplied together"},
		{name: "missing window start", query: "inflation", filters: SimilarSourceRecordFilters{PublicationWindowEnd: semanticTime(time.Now())}, limit: 1, contains: "start and end must be supplied together"},
		{name: "zero window start", query: "inflation", filters: semanticWindow(time.Time{}, time.Now()), limit: 1, contains: "start is required"},
		{name: "zero window end", query: "inflation", filters: semanticWindow(time.Now(), time.Time{}), limit: 1, contains: "end is required"},
		{name: "reversed window", query: "inflation", filters: semanticWindow(time.Now(), time.Now().Add(-time.Hour)), limit: 1, contains: "end must not be before start"},
		{name: "zero limit", query: "inflation", limit: 0, contains: "limit must be between"},
		{name: "negative limit", query: "inflation", limit: -1, contains: "limit must be between"},
		{name: "high limit", query: "inflation", limit: MaxSimilarSourceRecordsLimit + 1, contains: "limit must be between"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SearchSourceRecords(
				t.Context(),
				panicEmbedder{},
				panicSimilarSourceRecordReader{},
				test.query,
				test.filters,
				test.limit,
			)
			if err == nil || !strings.Contains(err.Error(), "validate semantic source record search") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("SearchSourceRecords() error = %v, want validation containing %q", err, test.contains)
			}
			if got != nil {
				t.Errorf("SearchSourceRecords() = %#v, want nil result", got)
			}
		})
	}
}

func TestSearchSourceRecordsRejectsMalformedQueryEmbedding(t *testing.T) {
	valid := EmbeddingBatch{
		Provider: "provider",
		Model:    "model",
		Embeddings: []ProviderEmbedding{{
			SourceRecordID: semanticSearchQueryID,
			Vector:         []float32{1, 2},
		}},
	}
	tests := []struct {
		name     string
		batch    EmbeddingBatch
		contains string
	}{
		{
			name:     "missing result",
			batch:    EmbeddingBatch{Provider: "provider", Model: "model"},
			contains: "returned 0 embeddings for 1 source records",
		},
		{
			name: "wrong identity",
			batch: EmbeddingBatch{Provider: "provider", Model: "model", Embeddings: []ProviderEmbedding{{
				SourceRecordID: "different-query",
				Vector:         []float32{1, 2},
			}},
			},
			contains: "does not match input ID",
		},
		{
			name: "invalid vector",
			batch: EmbeddingBatch{Provider: "provider", Model: "model", Embeddings: []ProviderEmbedding{{
				SourceRecordID: semanticSearchQueryID,
				Vector:         []float32{0, 0},
			}},
			},
			contains: "finite non-zero cosine norm",
		},
		{
			name:     "blank provenance",
			batch:    EmbeddingBatch{Provider: " ", Model: valid.Model, Embeddings: valid.Embeddings},
			contains: "provider is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SearchSourceRecords(
				t.Context(),
				&embedderStub{batch: test.batch},
				panicSimilarSourceRecordReader{},
				"query",
				SimilarSourceRecordFilters{},
				1,
			)
			if err == nil || !strings.Contains(err.Error(), "validate semantic search query embedding") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("SearchSourceRecords() error = %v, want validation containing %q", err, test.contains)
			}
			if got != nil {
				t.Errorf("SearchSourceRecords() = %#v, want nil result", got)
			}
		})
	}
}

func TestSearchSourceRecordsPreservesDependencyFailures(t *testing.T) {
	tests := []struct {
		name        string
		providerErr error
		readerErr   error
		contains    string
	}{
		{name: "provider", providerErr: errors.New("provider unavailable"), contains: "embed semantic search query with provider"},
		{name: "provider cancellation", providerErr: context.Canceled, contains: "embed semantic search query with provider"},
		{name: "repository", readerErr: errors.New("database unavailable"), contains: "retrieve similar source records"},
		{name: "repository cancellation", readerErr: context.Canceled, contains: "retrieve similar source records"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			embedder := &embedderStub{batch: validSemanticQueryBatch(), err: test.providerErr}
			var reader SimilarSourceRecordReader = &similarSourceRecordReaderStub{err: test.readerErr}
			if test.providerErr != nil {
				reader = panicSimilarSourceRecordReader{}
			}

			got, err := SearchSourceRecords(t.Context(), embedder, reader, "query", SimilarSourceRecordFilters{}, 1)
			wantErr := test.providerErr
			if wantErr == nil {
				wantErr = test.readerErr
			}
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("SearchSourceRecords() error = %v, want contextual %v", err, wantErr)
			}
			if got != nil {
				t.Errorf("SearchSourceRecords() = %#v, want nil result", got)
			}
		})
	}
}

func TestSearchSourceRecordsPreservesNonNilEmptyResults(t *testing.T) {
	want := []SimilarSourceRecord{}
	got, err := SearchSourceRecords(
		t.Context(),
		&embedderStub{batch: validSemanticQueryBatch()},
		&similarSourceRecordReaderStub{results: want},
		"query",
		SimilarSourceRecordFilters{},
		1,
	)
	if err != nil {
		t.Fatalf("SearchSourceRecords() error = %v", err)
	}
	if got == nil || !reflect.DeepEqual(got, want) {
		t.Errorf("SearchSourceRecords() = %#v, want non-nil empty result", got)
	}
}

func validSemanticQueryBatch() EmbeddingBatch {
	return EmbeddingBatch{
		Provider: "provider",
		Model:    "model",
		Embeddings: []ProviderEmbedding{{
			SourceRecordID: semanticSearchQueryID,
			Vector:         []float32{1, 2},
		}},
	}
}

func semanticSource(value string) *string {
	return &value
}

func semanticTime(value time.Time) *time.Time {
	return &value
}

func semanticWindow(windowStart, windowEnd time.Time) SimilarSourceRecordFilters {
	return SimilarSourceRecordFilters{
		PublicationWindowStart: semanticTime(windowStart),
		PublicationWindowEnd:   semanticTime(windowEnd),
	}
}

func semanticSearchResultFixture(id, title string, distance float64) SimilarSourceRecord {
	publishedAt := time.Date(2026, time.July, 12, 10, 0, 0, 0, time.UTC)
	return SimilarSourceRecord{
		SourceRecord: ingestion.StoredSourceRecord{
			ID: id,
			SourceRecord: ingestion.SourceRecord{
				Source:       "publisher",
				SourceItemID: "item-" + id,
				OriginalURL:  "https://example.com/" + id,
				Title:        title,
				PublishedAt:  publishedAt,
				RetrievedAt:  publishedAt.Add(time.Hour),
			},
			CreatedAt: publishedAt.Add(2 * time.Hour),
			UpdatedAt: publishedAt.Add(3 * time.Hour),
			CreatedBy: "ingestion",
			UpdatedBy: "refresh",
		},
		Provider:       "openai",
		Model:          "embedding-model",
		CosineDistance: distance,
	}
}

type similarSourceRecordReaderStub struct {
	results     []SimilarSourceRecord
	err         error
	calls       int
	provider    string
	model       string
	queryVector []float32
	filters     SimilarSourceRecordFilters
	limit       int
}

func (reader *similarSourceRecordReaderStub) SimilarSourceRecords(
	_ context.Context,
	provider string,
	model string,
	queryVector []float32,
	filters SimilarSourceRecordFilters,
	limit int,
) ([]SimilarSourceRecord, error) {
	reader.calls++
	reader.provider = provider
	reader.model = model
	reader.queryVector = queryVector
	reader.filters = copySemanticFilters(filters)
	reader.limit = limit
	return reader.results, reader.err
}

type panicSimilarSourceRecordReader struct{}

func (panicSimilarSourceRecordReader) SimilarSourceRecords(
	context.Context,
	string,
	string,
	[]float32,
	SimilarSourceRecordFilters,
	int,
) ([]SimilarSourceRecord, error) {
	panic("similar source record retrieval must not run")
}

func copySemanticFilters(filters SimilarSourceRecordFilters) SimilarSourceRecordFilters {
	if filters.Source != nil {
		copiedSource := *filters.Source
		filters.Source = &copiedSource
	}
	if filters.PublicationWindowStart != nil {
		copiedStart := *filters.PublicationWindowStart
		filters.PublicationWindowStart = &copiedStart
	}
	if filters.PublicationWindowEnd != nil {
		copiedEnd := *filters.PublicationWindowEnd
		filters.PublicationWindowEnd = &copiedEnd
	}
	return filters
}
