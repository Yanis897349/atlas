package search

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestEmbedSourceRecordsPreservesOrderIdentityAndProvenance(t *testing.T) {
	records := []ingestion.StoredSourceRecord{
		{ID: "record-second", SourceRecord: ingestion.SourceRecord{Source: "publisher-b", Title: "  Title kept exactly  "}},
		{ID: "record-first", SourceRecord: ingestion.SourceRecord{Source: "publisher-a", Title: "Another headline"}},
	}
	embedder := &embedderStub{batch: EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []ProviderEmbedding{
			{SourceRecordID: "record-second", Vector: []float32{0.1, 0.2, 0.3}},
			{SourceRecordID: "record-first", Vector: []float32{0.4, 0.5, 0.6}},
		},
	}}

	got, err := EmbedSourceRecords(t.Context(), embedder, records)
	if err != nil {
		t.Fatalf("EmbedSourceRecords() error = %v", err)
	}
	wantInputs := []EmbeddingInput{
		{SourceRecordID: "record-second", Text: "  Title kept exactly  "},
		{SourceRecordID: "record-first", Text: "Another headline"},
	}
	want := []SourceRecordEmbedding{
		{SourceRecordID: "record-second", Provider: "openai", Model: "embedding-model", Vector: []float32{0.1, 0.2, 0.3}},
		{SourceRecordID: "record-first", Provider: "openai", Model: "embedding-model", Vector: []float32{0.4, 0.5, 0.6}},
	}
	if embedder.calls != 1 || !reflect.DeepEqual(embedder.inputs, wantInputs) {
		t.Errorf("embedder call = (%d, %#v), want (1, %#v)", embedder.calls, embedder.inputs, wantInputs)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EmbedSourceRecords() = %#v, want %#v", got, want)
	}
}

func TestEmbedSourceRecordsReturnsEmptyWithoutProviderCall(t *testing.T) {
	got, err := EmbedSourceRecords(t.Context(), panicEmbedder{}, nil)
	if err != nil {
		t.Fatalf("EmbedSourceRecords() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("EmbedSourceRecords() = %#v, want non-nil empty result", got)
	}
}

func TestEmbedSourceRecordsRejectsInvalidInputsBeforeProviderCall(t *testing.T) {
	valid := []ingestion.StoredSourceRecord{
		{ID: "record-1", SourceRecord: ingestion.SourceRecord{Title: "Headline one"}},
		{ID: "record-2", SourceRecord: ingestion.SourceRecord{Title: "Headline two"}},
	}
	tests := []struct {
		name     string
		records  []ingestion.StoredSourceRecord
		contains string
	}{
		{name: "blank ID", records: withRecord(valid, 0, func(record *ingestion.StoredSourceRecord) { record.ID = " \t" }), contains: "source record 0 ID is required"},
		{name: "blank title", records: withRecord(valid, 1, func(record *ingestion.StoredSourceRecord) { record.Title = "\n" }), contains: "source record 1 title is required"},
		{name: "duplicate ID", records: withRecord(valid, 1, func(record *ingestion.StoredSourceRecord) { record.ID = "record-1" }), contains: "source record 1 ID \"record-1\" is duplicated"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EmbedSourceRecords(t.Context(), panicEmbedder{}, test.records)
			if err == nil || !strings.Contains(err.Error(), "validate source record embedding input") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("EmbedSourceRecords() error = %v, want input validation containing %q", err, test.contains)
			}
			if got != nil {
				t.Errorf("EmbedSourceRecords() = %#v, want nil result", got)
			}
		})
	}
}

func TestEmbedSourceRecordsPreservesProviderFailures(t *testing.T) {
	for _, wantErr := range []error{errors.New("provider unavailable"), context.Canceled} {
		got, err := EmbedSourceRecords(
			t.Context(),
			&embedderStub{err: wantErr},
			[]ingestion.StoredSourceRecord{{ID: "record-1", SourceRecord: ingestion.SourceRecord{Title: "Headline"}}},
		)
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "embed source records with provider") {
			t.Fatalf("EmbedSourceRecords() error = %v, want contextual %v", err, wantErr)
		}
		if got != nil {
			t.Errorf("EmbedSourceRecords() = %#v, want nil result", got)
		}
	}
}

func TestEmbedSourceRecordsRejectsInvalidProviderBatches(t *testing.T) {
	records := []ingestion.StoredSourceRecord{
		{ID: "record-1", SourceRecord: ingestion.SourceRecord{Title: "Headline one"}},
		{ID: "record-2", SourceRecord: ingestion.SourceRecord{Title: "Headline two"}},
	}
	valid := EmbeddingBatch{
		Provider: "provider",
		Model:    "model",
		Embeddings: []ProviderEmbedding{
			{SourceRecordID: "record-1", Vector: []float32{0.1, 0.2}},
			{SourceRecordID: "record-2", Vector: []float32{0.3, 0.4}},
		},
	}
	tests := []struct {
		name     string
		batch    EmbeddingBatch
		contains string
	}{
		{name: "blank provider", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Provider = " " }), contains: "provider is required"},
		{name: "blank model", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Model = "\t" }), contains: "model is required"},
		{name: "missing result", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Embeddings = batch.Embeddings[:1] }), contains: "returned 1 embeddings for 2 source records"},
		{name: "extra result", batch: withBatch(valid, func(batch *EmbeddingBatch) {
			batch.Embeddings = append(batch.Embeddings, ProviderEmbedding{SourceRecordID: "record-3", Vector: []float32{0.5, 0.6}})
		}), contains: "returned 3 embeddings for 2 source records"},
		{name: "reordered results", batch: withBatch(valid, func(batch *EmbeddingBatch) {
			batch.Embeddings[0], batch.Embeddings[1] = batch.Embeddings[1], batch.Embeddings[0]
		}), contains: "embedding 0 source record ID \"record-2\" does not match input ID \"record-1\""},
		{name: "substituted ID", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Embeddings[1].SourceRecordID = "record-3" }), contains: "embedding 1 source record ID \"record-3\" does not match input ID \"record-2\""},
		{name: "duplicate result", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Embeddings[1].SourceRecordID = "record-1" }), contains: "embedding 1 source record ID \"record-1\" does not match input ID \"record-2\""},
		{name: "empty vector", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Embeddings[0].Vector = nil }), contains: "embedding 0 vector is required"},
		{name: "zero norm", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Embeddings[0].Vector = []float32{0, 0} }), contains: "embedding 0 vector must have finite non-zero cosine norm"},
		{name: "NaN", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Embeddings[0].Vector[1] = float32(math.NaN()) }), contains: "embedding 0 vector value 1 must be finite"},
		{name: "dimension mismatch", batch: withBatch(valid, func(batch *EmbeddingBatch) { batch.Embeddings[1].Vector = []float32{0.3} }), contains: "embedding 1 vector dimension 1 does not match batch dimension 2"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EmbedSourceRecords(t.Context(), &embedderStub{batch: test.batch}, records)
			if err == nil || !strings.Contains(err.Error(), "validate source record embeddings") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("EmbedSourceRecords() error = %v, want provider validation containing %q", err, test.contains)
			}
			if got != nil {
				t.Errorf("EmbedSourceRecords() = %#v, want nil result", got)
			}
		})
	}
}

func withRecord(
	records []ingestion.StoredSourceRecord,
	index int,
	update func(*ingestion.StoredSourceRecord),
) []ingestion.StoredSourceRecord {
	result := append([]ingestion.StoredSourceRecord(nil), records...)
	update(&result[index])
	return result
}

func withBatch(batch EmbeddingBatch, update func(*EmbeddingBatch)) EmbeddingBatch {
	batch.Embeddings = append([]ProviderEmbedding(nil), batch.Embeddings...)
	for index := range batch.Embeddings {
		batch.Embeddings[index].Vector = append([]float32(nil), batch.Embeddings[index].Vector...)
	}
	update(&batch)
	return batch
}
