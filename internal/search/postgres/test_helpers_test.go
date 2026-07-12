package postgres

import (
	"testing"
	"time"

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
