package postgres

import (
	"errors"
	"testing"

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

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
