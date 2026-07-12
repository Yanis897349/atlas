package postgres

import (
	"context"
	"fmt"

	recordpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres/record"
	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// SimilarSourceRecords returns exact cosine-distance matches for one provider and model.
func (repository *Repository) SimilarSourceRecords(
	ctx context.Context,
	provider string,
	model string,
	queryVector []float32,
	limit int,
) ([]search.SimilarSourceRecord, error) {
	provider, model, err := normalizeAndValidateSimilarityQuery(provider, model, queryVector, limit)
	if err != nil {
		return nil, err
	}

	rows, err := repository.db.Query(
		ctx,
		similarSourceRecordsSQL,
		provider,
		model,
		pgvector.NewVector(queryVector),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query similar source records: %w", err)
	}
	defer rows.Close()

	results := make([]search.SimilarSourceRecord, 0, limit)
	for rows.Next() {
		result, scanErr := scanSimilarSourceRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan similar source record: %w", scanErr)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate similar source records: %w", err)
	}
	return results, nil
}

func scanSimilarSourceRecord(row pgx.Row) (search.SimilarSourceRecord, error) {
	var result search.SimilarSourceRecord
	destinations := recordpostgres.Destinations(&result.SourceRecord)
	destinations = append(destinations, &result.Provider, &result.Model, &result.CosineDistance)
	if err := row.Scan(destinations...); err != nil {
		return search.SimilarSourceRecord{}, err
	}
	recordpostgres.NormalizeTimes(&result.SourceRecord)
	return result, nil
}

var similarSourceRecordsSQL = `
WITH matching_embeddings AS MATERIALIZED (
    SELECT source_record_id, provider, model, embedding
    FROM source_record_embeddings
    WHERE provider = $1
      AND model = $2
      AND public.vector_dims(embedding) = public.vector_dims($3::public.vector)
      AND (embedding OPERATOR(public.<=>) embedding) = 0
)
SELECT
` + recordpostgres.Columns("source_records") + `,
    matching_embeddings.provider,
    matching_embeddings.model,
    matching_embeddings.embedding OPERATOR(public.<=>) $3::public.vector AS cosine_distance
FROM matching_embeddings
JOIN source_records ON source_records.id = matching_embeddings.source_record_id
ORDER BY cosine_distance, source_records.id
LIMIT $4`
