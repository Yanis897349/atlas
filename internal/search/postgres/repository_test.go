package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestEmbeddingMigrationsAreRepeatableAndCreateRequiredSchema(t *testing.T) {
	pool := openTestPool(t)
	if err := databasepostgres.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("repeat Migrate() error = %v", err)
	}

	var extension, table, provenanceIndex, cosineNormConstraint string
	if err := pool.QueryRow(t.Context(), `SELECT extname FROM pg_extension WHERE extname = 'vector'`).Scan(&extension); err != nil {
		t.Fatalf("load vector extension: %v", err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT to_regclass('source_record_embeddings')::text`).Scan(&table); err != nil {
		t.Fatalf("load embedding table: %v", err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT to_regclass('ix_source_record_embeddings_provider_model')::text`).Scan(&provenanceIndex); err != nil {
		t.Fatalf("load embedding provenance index: %v", err)
	}
	if err := pool.QueryRow(t.Context(), `
SELECT conname
FROM pg_constraint
WHERE conrelid = 'source_record_embeddings'::regclass
  AND conname = 'chk_source_record_embeddings_embedding_cosine_norm'
`).Scan(&cosineNormConstraint); err != nil {
		t.Fatalf("load embedding cosine-norm constraint: %v", err)
	}
	if extension != "vector" || table != "source_record_embeddings" ||
		provenanceIndex != "ix_source_record_embeddings_provider_model" ||
		cosineNormConstraint != "chk_source_record_embeddings_embedding_cosine_norm" {
		t.Errorf("migration schema = (%q, %q, %q, %q)", extension, table, provenanceIndex, cosineNormConstraint)
	}
}

func TestEmbeddingMigrationRejectsInvalidCosineNorms(t *testing.T) {
	for _, test := range []struct {
		name   string
		vector string
	}{
		{name: "zero", vector: "[0,0]"},
		{name: "underflow", vector: "[1e-30]"},
		{name: "overflow", vector: "[1e30]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool := openTestPool(t)
			recordID := insertSourceRecord(t, pool, "embedding-invalid-cosine-norm-"+test.name)

			_, err := pool.Exec(t.Context(), `
INSERT INTO source_record_embeddings (
    source_record_id, provider, model, embedding, created_by, updated_by
)
VALUES ($1, 'openai', 'model-a', $2::public.vector, 'test', 'test')
`, recordID, test.vector)
			var postgresError *pgconn.PgError
			if !errors.As(err, &postgresError) || postgresError.Code != "23514" ||
				postgresError.ConstraintName != "chk_source_record_embeddings_embedding_cosine_norm" {
				t.Fatalf("invalid cosine-norm embedding error = %v, want cosine-norm constraint violation", err)
			}
			assertEmbeddingCount(t, pool, 0)
		})
	}
}

type storedEmbedding struct {
	ID             string
	SourceRecordID string
	Provider       string
	Model          string
	Vector         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CreatedBy      string
	UpdatedBy      string
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	database := postgrestest.Open(t)
	if err := databasepostgres.Migrate(t.Context(), database.Pool); err != nil {
		t.Fatalf("apply database migrations: %v", err)
	}
	return database.Pool
}

func insertSourceRecord(t *testing.T, pool *pgxpool.Pool, sourceItemID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `
INSERT INTO source_records (
    source, source_item_id, original_url, title, published_at, retrieved_at, created_by, updated_by
)
VALUES ('test-source', $1, 'https://example.com/source', 'Source title', statement_timestamp(), statement_timestamp(), 'test', 'test')
RETURNING id::text
`, sourceItemID).Scan(&id); err != nil {
		t.Fatalf("insert source record: %v", err)
	}
	return id
}

func loadEmbeddings(t *testing.T, pool *pgxpool.Pool) []storedEmbedding {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
SELECT id::text, source_record_id::text, provider, model, embedding::text,
       created_at, updated_at, created_by, updated_by
FROM source_record_embeddings
ORDER BY source_record_id, provider, model
`)
	if err != nil {
		t.Fatalf("load embeddings: %v", err)
	}
	defer rows.Close()
	stored, err := pgx.CollectRows(rows, pgx.RowToStructByPos[storedEmbedding])
	if err != nil {
		t.Fatalf("scan embeddings: %v", err)
	}
	return stored
}

func loadEmbedding(t *testing.T, pool *pgxpool.Pool, sourceRecordID string) storedEmbedding {
	t.Helper()
	for _, stored := range loadEmbeddings(t, pool) {
		if stored.SourceRecordID == sourceRecordID {
			return stored
		}
	}
	t.Fatalf("embedding for source record %q not found", sourceRecordID)
	return storedEmbedding{}
}

func embeddingByRecordID(t *testing.T, embeddings []storedEmbedding, sourceRecordID string) storedEmbedding {
	t.Helper()
	for _, stored := range embeddings {
		if stored.SourceRecordID == sourceRecordID {
			return stored
		}
	}
	t.Fatalf("embedding for source record %q not found", sourceRecordID)
	return storedEmbedding{}
}

func assertEmbeddingCount(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM source_record_embeddings`).Scan(&count); err != nil {
		t.Fatalf("count source record embeddings: %v", err)
	}
	if count != want {
		t.Errorf("source record embedding count = %d, want %d", count, want)
	}
}
