package postgres

import (
	"math"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	recordpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres/record"
	"github.com/Yanis897349/atlas/internal/search"
)

func TestRepositoryRetrievesSimilarSourceRecords(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := NewRepository(pool)
	sourceRepository, _ := ingestionpostgres.NewRepository(pool)
	publishedAt := time.Date(2026, time.July, 12, 10, 0, 0, 0, time.UTC)
	records := make([]ingestion.StoredSourceRecord, 0, 7)
	for index, itemID := range []string{"tie-a", "tie-b", "orthogonal", "opposite", "other-provider", "other-model", "other-dimension"} {
		source := "test-source"
		if itemID == "tie-b" {
			source = "other-source"
		}
		record, err := sourceRepository.UpsertSourceRecord(t.Context(), ingestion.SourceRecord{
			Source:       source,
			SourceItemID: itemID,
			OriginalURL:  "https://example.com/" + itemID,
			Title:        "Title " + itemID,
			PublishedAt:  publishedAt.Add(time.Duration(index) * time.Minute),
			RetrievedAt:  publishedAt.Add(time.Hour),
		}, "source-creator")
		if err != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", itemID, err)
		}
		records = append(records, record)
	}

	persistSimilarityEmbeddings(t, repository, records[:4], "openai", "model-a", [][]float32{
		{1, 0}, {2, 0}, {0, 1}, {-1, 0},
	})
	persistSimilarityEmbeddings(t, repository, records[4:5], "other-provider", "model-a", [][]float32{{1, 0}})
	persistSimilarityEmbeddings(t, repository, records[5:6], "openai", "other-model", [][]float32{{1, 0}})
	persistSimilarityEmbeddings(t, repository, records[6:7], "openai", "model-a", [][]float32{{1, 0, 0}})

	got, err := repository.SimilarSourceRecords(t.Context(), " openai ", " model-a ", []float32{1, 0}, nil, 10)
	if err != nil {
		t.Fatalf("SimilarSourceRecords() error = %v", err)
	}
	tieIDs := []string{records[0].ID, records[1].ID}
	sort.Strings(tieIDs)
	wantIDs := []string{tieIDs[0], tieIDs[1], records[2].ID, records[3].ID}
	wantDistances := []float64{0, 0, 1, 2}
	if len(got) != len(wantIDs) {
		t.Fatalf("SimilarSourceRecords() count = %d, want %d", len(got), len(wantIDs))
	}
	recordByID := make(map[string]ingestion.StoredSourceRecord, len(records))
	for _, record := range records {
		recordpostgres.NormalizeTimes(&record)
		recordByID[record.ID] = record
	}
	for index, result := range got {
		if result.SourceRecord.ID != wantIDs[index] {
			t.Errorf("SimilarSourceRecords()[%d].ID = %q, want %q", index, result.SourceRecord.ID, wantIDs[index])
		}
		if !reflect.DeepEqual(result.SourceRecord, recordByID[result.SourceRecord.ID]) {
			t.Errorf("SimilarSourceRecords()[%d].SourceRecord = %#v, want %#v", index, result.SourceRecord, recordByID[result.SourceRecord.ID])
		}
		for _, timestamp := range []time.Time{
			result.SourceRecord.PublishedAt,
			result.SourceRecord.RetrievedAt,
			result.SourceRecord.CreatedAt,
			result.SourceRecord.UpdatedAt,
		} {
			if timestamp.Location() != time.UTC {
				t.Errorf("SimilarSourceRecords()[%d] timestamp location = %v, want UTC", index, timestamp.Location())
			}
		}
		if result.Provider != "openai" || result.Model != "model-a" {
			t.Errorf("SimilarSourceRecords()[%d] provenance = (%q, %q)", index, result.Provider, result.Model)
		}
		if math.Abs(result.CosineDistance-wantDistances[index]) > 1e-12 {
			t.Errorf("SimilarSourceRecords()[%d].CosineDistance = %v, want %v", index, result.CosineDistance, wantDistances[index])
		}
	}

	limited, err := repository.SimilarSourceRecords(t.Context(), "openai", "model-a", []float32{1, 0}, nil, 2)
	if err != nil {
		t.Fatalf("limited SimilarSourceRecords() error = %v", err)
	}
	if len(limited) != 2 || limited[0].SourceRecord.ID != tieIDs[0] || limited[1].SourceRecord.ID != tieIDs[1] {
		t.Errorf("limited SimilarSourceRecords() = %#v, want UUID-ordered tied results", limited)
	}

	source := "  test-source  "
	filtered, err := repository.SimilarSourceRecords(t.Context(), "openai", "model-a", []float32{1, 0}, &source, 10)
	if err != nil {
		t.Fatalf("filtered SimilarSourceRecords() error = %v", err)
	}
	wantFilteredIDs := []string{records[0].ID, records[2].ID, records[3].ID}
	if len(filtered) != len(wantFilteredIDs) {
		t.Fatalf("filtered SimilarSourceRecords() count = %d, want %d", len(filtered), len(wantFilteredIDs))
	}
	for index, wantID := range wantFilteredIDs {
		if filtered[index].SourceRecord.ID != wantID || filtered[index].SourceRecord.Source != "test-source" {
			t.Errorf("filtered SimilarSourceRecords()[%d] = %#v, want source record %q", index, filtered[index], wantID)
		}
	}

	caseSensitiveSource := "TEST-SOURCE"
	caseSensitive, err := repository.SimilarSourceRecords(
		t.Context(), "openai", "model-a", []float32{1, 0}, &caseSensitiveSource, 10,
	)
	if err != nil {
		t.Fatalf("case-sensitive SimilarSourceRecords() error = %v", err)
	}
	if caseSensitive == nil || len(caseSensitive) != 0 {
		t.Errorf("case-sensitive SimilarSourceRecords() = %#v, want non-nil empty result", caseSensitive)
	}
}

func TestRepositoryReturnsEmptySimilarSourceRecords(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := NewRepository(pool)

	got, err := repository.SimilarSourceRecords(t.Context(), "openai", "model-a", []float32{1, 0}, nil, 10)
	if err != nil {
		t.Fatalf("SimilarSourceRecords() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("SimilarSourceRecords() = %#v, want non-nil empty result", got)
	}
}

func persistSimilarityEmbeddings(
	t *testing.T,
	repository *Repository,
	records []ingestion.StoredSourceRecord,
	provider string,
	model string,
	vectors [][]float32,
) {
	t.Helper()
	embeddings := make([]search.SourceRecordEmbedding, len(records))
	for index, record := range records {
		embeddings[index] = search.SourceRecordEmbedding{
			SourceRecordID: record.ID,
			Provider:       provider,
			Model:          model,
			Vector:         vectors[index],
		}
	}
	if err := repository.PersistSourceRecordEmbeddings(t.Context(), embeddings, "embedding-creator"); err != nil {
		t.Fatalf("PersistSourceRecordEmbeddings() error = %v", err)
	}
}
