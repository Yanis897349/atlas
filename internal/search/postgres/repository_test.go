package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/search"
)

func TestRepositoryPersistsRetriesAndReplacesEmbeddings(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := NewRepository(pool)
	recordIDs := []string{
		insertSourceRecord(t, pool, "embedding-first"),
		insertSourceRecord(t, pool, "embedding-second"),
	}
	embeddings := []search.SourceRecordEmbedding{
		{SourceRecordID: recordIDs[1], Provider: " openai ", Model: " model-a ", Vector: []float32{0.3, 0.4}},
		{SourceRecordID: recordIDs[0], Provider: "openai", Model: "model-a", Vector: []float32{0.1, 0.2}},
	}

	if err := repository.PersistSourceRecordEmbeddings(t.Context(), embeddings, " creator "); err != nil {
		t.Fatalf("PersistSourceRecordEmbeddings() error = %v", err)
	}
	created := loadEmbeddings(t, pool)
	if len(created) != 2 {
		t.Fatalf("embedding count = %d, want 2", len(created))
	}
	for _, stored := range created {
		if stored.Provider != "openai" || stored.Model != "model-a" ||
			stored.CreatedBy != "creator" || stored.UpdatedBy != "creator" {
			t.Errorf("stored embedding = %#v", stored)
		}
	}

	if err := repository.PersistSourceRecordEmbeddings(t.Context(), embeddings, "retry-actor"); err != nil {
		t.Fatalf("retry PersistSourceRecordEmbeddings() error = %v", err)
	}
	retried := loadEmbeddings(t, pool)
	if !reflect.DeepEqual(retried, created) {
		t.Errorf("retried embeddings = %#v, want unchanged %#v", retried, created)
	}

	if _, err := pool.Exec(t.Context(), `SELECT pg_sleep(0.01)`); err != nil {
		t.Fatalf("wait before update: %v", err)
	}
	updatedInput := []search.SourceRecordEmbedding{
		{SourceRecordID: recordIDs[0], Provider: "openai", Model: "model-a", Vector: []float32{0.8, 0.9}},
	}
	if err := repository.PersistSourceRecordEmbeddings(t.Context(), updatedInput, "updater"); err != nil {
		t.Fatalf("update PersistSourceRecordEmbeddings() error = %v", err)
	}
	updated := loadEmbedding(t, pool, recordIDs[0])
	original := embeddingByRecordID(t, created, recordIDs[0])
	if updated.ID != original.ID || updated.CreatedAt != original.CreatedAt || updated.CreatedBy != original.CreatedBy {
		t.Errorf("creation metadata changed from %#v to %#v", original, updated)
	}
	if updated.Vector != "[0.8,0.9]" || !updated.UpdatedAt.After(original.UpdatedAt) || updated.UpdatedBy != "updater" {
		t.Errorf("updated embedding = %#v", updated)
	}
	assertEmbeddingCount(t, pool, 2)
}

func TestRepositoryRollsBackEmbeddingBatch(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := NewRepository(pool)
	recordID := insertSourceRecord(t, pool, "embedding-rollback")
	embeddings := []search.SourceRecordEmbedding{
		{SourceRecordID: recordID, Provider: "openai", Model: "model-a", Vector: []float32{0.1}},
		{SourceRecordID: "ffffffff-ffff-ffff-ffff-ffffffffffff", Provider: "openai", Model: "model-a", Vector: []float32{0.2}},
	}

	err := repository.PersistSourceRecordEmbeddings(t.Context(), embeddings, "creator")
	if err == nil || !strings.Contains(err.Error(), "persist source record embedding") {
		t.Fatalf("PersistSourceRecordEmbeddings() error = %v, want contextual reference failure", err)
	}
	assertEmbeddingCount(t, pool, 0)
}

func TestRepositoryCascadesEmbeddingsWithSourceRecord(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := NewRepository(pool)
	recordID := insertSourceRecord(t, pool, "embedding-cascade")
	if err := repository.PersistSourceRecordEmbeddings(t.Context(), []search.SourceRecordEmbedding{{
		SourceRecordID: recordID, Provider: "openai", Model: "model-a", Vector: []float32{0.1},
	}}, "creator"); err != nil {
		t.Fatalf("PersistSourceRecordEmbeddings() error = %v", err)
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM source_records WHERE id = $1`, recordID); err != nil {
		t.Fatalf("delete source record: %v", err)
	}
	assertEmbeddingCount(t, pool, 0)
}

func TestRepositoryPreservesCancellationAndRollsBackCommitFailure(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := NewRepository(pool)
	recordID := insertSourceRecord(t, pool, "embedding-failures")
	embedding := []search.SourceRecordEmbedding{{
		SourceRecordID: recordID, Provider: "openai", Model: "model-a", Vector: []float32{0.1},
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := repository.PersistSourceRecordEmbeddings(ctx, embedding, "creator"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled PersistSourceRecordEmbeddings() error = %v, want context.Canceled", err)
	}

	if _, err := pool.Exec(t.Context(), `
CREATE FUNCTION reject_source_record_embedding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'reject source record embedding';
END;
$$;
CREATE CONSTRAINT TRIGGER reject_source_record_embedding
AFTER INSERT ON source_record_embeddings
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION reject_source_record_embedding()
`); err != nil {
		t.Fatalf("create deferred embedding failure: %v", err)
	}
	err := repository.PersistSourceRecordEmbeddings(t.Context(), embedding, "creator")
	if err == nil || !strings.Contains(err.Error(), "commit source record embedding persistence") {
		t.Fatalf("PersistSourceRecordEmbeddings() error = %v, want contextual commit failure", err)
	}
	assertEmbeddingCount(t, pool, 0)
}

func TestRepositorySerializesOverlappingEmbeddingBatches(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := NewRepository(pool)
	recordIDs := []string{
		insertSourceRecord(t, pool, "embedding-concurrent-first"),
		insertSourceRecord(t, pool, "embedding-concurrent-second"),
	}
	if _, err := pool.Exec(t.Context(), `
CREATE FUNCTION delay_source_record_embedding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_sleep(0.1);
    RETURN NEW;
END;
$$;
CREATE TRIGGER delay_source_record_embedding
AFTER INSERT ON source_record_embeddings
FOR EACH ROW EXECUTE FUNCTION delay_source_record_embedding()
`); err != nil {
		t.Fatalf("create embedding concurrency delay: %v", err)
	}
	batches := [][]search.SourceRecordEmbedding{
		{
			{SourceRecordID: recordIDs[0], Provider: "openai", Model: "model-a", Vector: []float32{0.1}},
			{SourceRecordID: recordIDs[1], Provider: "openai", Model: "model-a", Vector: []float32{0.2}},
		},
		{
			{SourceRecordID: recordIDs[1], Provider: "openai", Model: "model-a", Vector: []float32{0.2}},
			{SourceRecordID: recordIDs[0], Provider: "openai", Model: "model-a", Vector: []float32{0.1}},
		},
	}
	start := make(chan struct{})
	errorsChannel := make(chan error, len(batches))
	var waitGroup sync.WaitGroup
	for _, batch := range batches {
		waitGroup.Go(func() {
			<-start
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			errorsChannel <- repository.PersistSourceRecordEmbeddings(ctx, batch, "creator")
		})
	}
	close(start)
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Errorf("concurrent PersistSourceRecordEmbeddings() error = %v", err)
		}
	}
	assertEmbeddingCount(t, pool, 2)
}
