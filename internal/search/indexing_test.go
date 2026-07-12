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

func TestIndexSourceRecordEmbeddingsPreservesRetrievalOrderAndProvenance(t *testing.T) {
	windowStart := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.FixedZone("test", 2*60*60))
	windowEnd := windowStart.Add(12 * time.Hour)
	records := []ingestion.StoredSourceRecord{
		{ID: "record-second", SourceRecord: ingestion.SourceRecord{Title: "  Exact persisted title  "}},
		{ID: "record-first", SourceRecord: ingestion.SourceRecord{Title: "Earlier title"}},
	}
	reader := &indexSourceRecordReaderStub{records: records}
	embedder := &embedderStub{batch: EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []ProviderEmbedding{
			{SourceRecordID: "record-second", Vector: []float32{0.1, 0.2}},
			{SourceRecordID: "record-first", Vector: []float32{0.3, 0.4}},
		},
	}}
	writer := &sourceRecordEmbeddingWriterStub{}

	got, err := IndexSourceRecordEmbeddings(
		t.Context(),
		reader,
		embedder,
		writer,
		windowStart,
		windowEnd,
		17,
		" indexing-actor ",
	)
	if err != nil {
		t.Fatalf("IndexSourceRecordEmbeddings() error = %v", err)
	}

	wantInputs := []EmbeddingInput{
		{SourceRecordID: "record-second", Text: "  Exact persisted title  "},
		{SourceRecordID: "record-first", Text: "Earlier title"},
	}
	wantEmbeddings := []SourceRecordEmbedding{
		{SourceRecordID: "record-second", Provider: "openai", Model: "embedding-model", Vector: []float32{0.1, 0.2}},
		{SourceRecordID: "record-first", Provider: "openai", Model: "embedding-model", Vector: []float32{0.3, 0.4}},
	}
	if reader.calls != 1 || !reader.windowStart.Equal(windowStart) || !reader.windowEnd.Equal(windowEnd) || reader.limit != 17 {
		t.Errorf(
			"reader call = (%d, %v, %v, %d), want (1, %v, %v, 17)",
			reader.calls,
			reader.windowStart,
			reader.windowEnd,
			reader.limit,
			windowStart,
			windowEnd,
		)
	}
	if embedder.calls != 1 || !reflect.DeepEqual(embedder.inputs, wantInputs) {
		t.Errorf("embedder call = (%d, %#v), want (1, %#v)", embedder.calls, embedder.inputs, wantInputs)
	}
	if writer.calls != 1 || writer.actor != " indexing-actor " || !reflect.DeepEqual(writer.embeddings, wantEmbeddings) {
		t.Errorf(
			"writer call = (%d, %#v, %q), want (1, %#v, %q)",
			writer.calls,
			writer.embeddings,
			writer.actor,
			wantEmbeddings,
			" indexing-actor ",
		)
	}
	if !reflect.DeepEqual(got, wantEmbeddings) {
		t.Errorf("IndexSourceRecordEmbeddings() = %#v, want %#v", got, wantEmbeddings)
	}

	embedder.batch.Embeddings[0].Vector[0] = 99
	if got[0].Vector[0] != 0.1 || writer.embeddings[0].Vector[0] != 0.1 {
		t.Errorf("indexed vectors changed with provider result: result=%#v writer=%#v", got, writer.embeddings)
	}
}

func TestIndexSourceRecordEmbeddingsPersistsEmptyResultWithoutProviderCall(t *testing.T) {
	writer := &sourceRecordEmbeddingWriterStub{}

	got, err := IndexSourceRecordEmbeddings(
		t.Context(),
		&indexSourceRecordReaderStub{records: []ingestion.StoredSourceRecord{}},
		panicEmbedder{},
		writer,
		time.Now(),
		time.Now(),
		1,
		"actor",
	)
	if err != nil {
		t.Fatalf("IndexSourceRecordEmbeddings() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("IndexSourceRecordEmbeddings() = %#v, want non-nil empty result", got)
	}
	if writer.calls != 1 || writer.embeddings == nil || len(writer.embeddings) != 0 || writer.actor != "actor" {
		t.Errorf("writer call = (%d, %#v, %q), want one non-nil empty batch for actor", writer.calls, writer.embeddings, writer.actor)
	}
}

func TestIndexSourceRecordEmbeddingsPreservesDependencyFailures(t *testing.T) {
	dependencyError := errors.New("dependency unavailable")
	records := []ingestion.StoredSourceRecord{{
		ID:           "record-1",
		SourceRecord: ingestion.SourceRecord{Title: "Headline"},
	}}
	validBatch := EmbeddingBatch{
		Provider: "provider",
		Model:    "model",
		Embeddings: []ProviderEmbedding{{
			SourceRecordID: "record-1",
			Vector:         []float32{1, 2},
		}},
	}
	tests := []struct {
		name      string
		readerErr error
		embedErr  error
		writerErr error
		contains  string
	}{
		{name: "retrieval", readerErr: dependencyError, contains: "retrieve source records for embedding indexing"},
		{name: "retrieval cancellation", readerErr: context.Canceled, contains: "retrieve source records for embedding indexing"},
		{name: "provider", embedErr: dependencyError, contains: "embed source records for indexing"},
		{name: "provider cancellation", embedErr: context.Canceled, contains: "embed source records for indexing"},
		{name: "persistence", writerErr: dependencyError, contains: "persist indexed source record embeddings"},
		{name: "persistence cancellation", writerErr: context.Canceled, contains: "persist indexed source record embeddings"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &indexSourceRecordReaderStub{records: records, err: test.readerErr}
			embedder := &embedderStub{batch: validBatch, err: test.embedErr}
			writer := &sourceRecordEmbeddingWriterStub{err: test.writerErr}

			got, err := IndexSourceRecordEmbeddings(
				t.Context(),
				reader,
				embedder,
				writer,
				time.Now(),
				time.Now(),
				1,
				"actor",
			)
			wantErr := test.readerErr
			if wantErr == nil {
				wantErr = test.embedErr
			}
			if wantErr == nil {
				wantErr = test.writerErr
			}
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("IndexSourceRecordEmbeddings() error = %v, want contextual %v", err, wantErr)
			}
			if got != nil {
				t.Errorf("IndexSourceRecordEmbeddings() = %#v, want nil result", got)
			}
			if test.readerErr != nil && (embedder.calls != 0 || writer.calls != 0) {
				t.Errorf("calls after retrieval failure = (embedder %d, writer %d), want zero", embedder.calls, writer.calls)
			}
			if test.embedErr != nil && writer.calls != 0 {
				t.Errorf("writer calls after provider failure = %d, want zero", writer.calls)
			}
		})
	}
}

type indexSourceRecordReaderStub struct {
	records     []ingestion.StoredSourceRecord
	err         error
	calls       int
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (reader *indexSourceRecordReaderStub) RecentSourceRecords(
	_ context.Context,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]ingestion.StoredSourceRecord, error) {
	reader.calls++
	reader.windowStart = windowStart
	reader.windowEnd = windowEnd
	reader.limit = limit
	return reader.records, reader.err
}

type sourceRecordEmbeddingWriterStub struct {
	err        error
	calls      int
	embeddings []SourceRecordEmbedding
	actor      string
}

func (writer *sourceRecordEmbeddingWriterStub) PersistSourceRecordEmbeddings(
	_ context.Context,
	embeddings []SourceRecordEmbedding,
	actor string,
) error {
	writer.calls++
	writer.embeddings = cloneSourceRecordEmbeddings(embeddings)
	writer.actor = actor
	return writer.err
}

func cloneSourceRecordEmbeddings(embeddings []SourceRecordEmbedding) []SourceRecordEmbedding {
	cloned := make([]SourceRecordEmbedding, len(embeddings))
	for index, embedding := range embeddings {
		embedding.Vector = append([]float32(nil), embedding.Vector...)
		cloned[index] = embedding
	}
	return cloned
}
