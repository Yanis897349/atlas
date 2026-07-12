package search

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestIndexStoredSourceRecordsPreservesOrderTitlesAndProvenance(t *testing.T) {
	records := []ingestion.StoredSourceRecord{
		{ID: "record-2", SourceRecord: ingestion.SourceRecord{Title: "  Exact second title  "}},
		{ID: "record-1", SourceRecord: ingestion.SourceRecord{Title: "Exact first title"}},
	}
	embedder := &embedderStub{batch: EmbeddingBatch{
		Provider: " openai ",
		Model:    " model-a ",
		Embeddings: []ProviderEmbedding{
			{SourceRecordID: "record-2", Vector: []float32{1, 2}},
			{SourceRecordID: "record-1", Vector: []float32{3, 4}},
		},
	}}
	writer := &sourceRecordEmbeddingWriterStub{}

	got, err := IndexStoredSourceRecords(t.Context(), records, embedder, writer, "rss-actor")
	if err != nil {
		t.Fatalf("IndexStoredSourceRecords() error = %v", err)
	}
	wantInputs := []EmbeddingInput{
		{SourceRecordID: "record-2", Text: "  Exact second title  "},
		{SourceRecordID: "record-1", Text: "Exact first title"},
	}
	want := []SourceRecordEmbedding{
		{SourceRecordID: "record-2", Provider: "openai", Model: "model-a", Vector: []float32{1, 2}},
		{SourceRecordID: "record-1", Provider: "openai", Model: "model-a", Vector: []float32{3, 4}},
	}
	if !reflect.DeepEqual(embedder.inputs, wantInputs) {
		t.Errorf("embedder inputs = %#v, want %#v", embedder.inputs, wantInputs)
	}
	if !reflect.DeepEqual(writer.embeddings, want) || writer.actor != "rss-actor" {
		t.Errorf("writer call = (%#v, %q), want (%#v, rss-actor)", writer.embeddings, writer.actor, want)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("IndexStoredSourceRecords() = %#v, want %#v", got, want)
	}
}

func TestIndexStoredSourceRecordsSkipsProviderForEmptyInput(t *testing.T) {
	writer := &sourceRecordEmbeddingWriterStub{}
	got, err := IndexStoredSourceRecords(
		t.Context(),
		[]ingestion.StoredSourceRecord{},
		panicEmbedder{},
		writer,
		"rss-actor",
	)
	if err != nil {
		t.Fatalf("IndexStoredSourceRecords() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("IndexStoredSourceRecords() = %#v, want non-nil empty result", got)
	}
	if writer.calls != 1 || writer.embeddings == nil || len(writer.embeddings) != 0 {
		t.Errorf("writer calls = (%d, %#v), want one non-nil empty batch", writer.calls, writer.embeddings)
	}
}

func TestIndexStoredSourceRecordsPreservesFailures(t *testing.T) {
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
			Vector:         []float32{1},
		}},
	}
	tests := []struct {
		name      string
		embedErr  error
		writerErr error
		contains  string
	}{
		{name: "provider", embedErr: dependencyError, contains: "embed source records for indexing"},
		{name: "provider cancellation", embedErr: context.Canceled, contains: "embed source records for indexing"},
		{name: "persistence", writerErr: dependencyError, contains: "persist indexed source record embeddings"},
		{name: "persistence cancellation", writerErr: context.Canceled, contains: "persist indexed source record embeddings"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			embedder := &embedderStub{batch: validBatch, err: test.embedErr}
			writer := &sourceRecordEmbeddingWriterStub{err: test.writerErr}
			got, err := IndexStoredSourceRecords(t.Context(), records, embedder, writer, "actor")
			wantErr := test.embedErr
			if wantErr == nil {
				wantErr = test.writerErr
			}
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("IndexStoredSourceRecords() error = %v, want contextual %v", err, wantErr)
			}
			if got != nil {
				t.Errorf("IndexStoredSourceRecords() = %#v, want nil", got)
			}
			if test.embedErr != nil && writer.calls != 0 {
				t.Errorf("writer calls after provider failure = %d, want zero", writer.calls)
			}
		})
	}
}
