package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const semanticSearchQueryID = "semantic-search-query"

// SearchSourceRecords embeds one text query and retrieves similar canonical source records.
func SearchSourceRecords(
	ctx context.Context,
	embedder Embedder,
	reader SimilarSourceRecordReader,
	query string,
	limit int,
) ([]SimilarSourceRecord, error) {
	if err := validateSemanticSearchQuery(query, limit); err != nil {
		return nil, fmt.Errorf("validate semantic source record search: %w", err)
	}

	inputs := []EmbeddingInput{{SourceRecordID: semanticSearchQueryID, Text: query}}
	batch, err := embedder.Embed(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("embed semantic search query with provider: %w", err)
	}

	embeddings, err := validateEmbeddingBatch(inputs, batch)
	if err != nil {
		return nil, fmt.Errorf("validate semantic search query embedding: %w", err)
	}
	embedding := embeddings[0]

	results, err := reader.SimilarSourceRecords(
		ctx,
		embedding.Provider,
		embedding.Model,
		embedding.Vector,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieve similar source records: %w", err)
	}
	return results, nil
}

func validateSemanticSearchQuery(query string, limit int) error {
	if strings.TrimSpace(query) == "" {
		return errors.New("query is required")
	}
	if limit < 1 || limit > MaxSimilarSourceRecordsLimit {
		return fmt.Errorf("limit must be between 1 and %d", MaxSimilarSourceRecordsLimit)
	}
	return nil
}
