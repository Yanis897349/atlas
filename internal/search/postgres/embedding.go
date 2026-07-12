package postgres

import (
	"context"
	"fmt"
	"sort"

	"github.com/Yanis897349/atlas/internal/search"
	"github.com/pgvector/pgvector-go"
)

// PersistSourceRecordEmbeddings atomically inserts or replaces source-record embeddings.
func (repository *Repository) PersistSourceRecordEmbeddings(
	ctx context.Context,
	embeddings []search.SourceRecordEmbedding,
	actor string,
) error {
	embeddings, actor, err := normalizeAndValidateEmbeddings(embeddings, actor)
	if err != nil {
		return err
	}
	if len(embeddings) == 0 {
		return nil
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin source record embedding persistence: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	writeOrder := append([]search.SourceRecordEmbedding(nil), embeddings...)
	sort.Slice(writeOrder, func(left, right int) bool {
		if writeOrder[left].SourceRecordID != writeOrder[right].SourceRecordID {
			return writeOrder[left].SourceRecordID < writeOrder[right].SourceRecordID
		}
		if writeOrder[left].Provider != writeOrder[right].Provider {
			return writeOrder[left].Provider < writeOrder[right].Provider
		}
		return writeOrder[left].Model < writeOrder[right].Model
	})

	for index, embedding := range writeOrder {
		if _, err := transaction.Exec(
			ctx,
			upsertSourceRecordEmbeddingSQL,
			embedding.SourceRecordID,
			embedding.Provider,
			embedding.Model,
			pgvector.NewVector(embedding.Vector),
			actor,
		); err != nil {
			return fmt.Errorf(
				"persist source record embedding %d for source record %q: %w",
				index,
				embedding.SourceRecordID,
				err,
			)
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit source record embedding persistence: %w", err)
	}
	return nil
}

const upsertSourceRecordEmbeddingSQL = `
INSERT INTO source_record_embeddings (
    source_record_id,
    provider,
    model,
    embedding,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (source_record_id, provider, model) DO UPDATE
SET embedding = EXCLUDED.embedding,
    updated_at = statement_timestamp(),
    updated_by = EXCLUDED.updated_by
WHERE source_record_embeddings.embedding::text IS DISTINCT FROM EXCLUDED.embedding::text`
