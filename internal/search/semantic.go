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
	source *string,
	limit int,
) ([]SimilarSourceRecord, error) {
	source, err := normalizeAndValidateSemanticSearchQuery(query, source, limit)
	if err != nil {
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
		source,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieve similar source records: %w", err)
	}
	return results, nil
}

func normalizeAndValidateSemanticSearchQuery(query string, source *string, limit int) (*string, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query is required")
	}
	if source != nil {
		normalizedSource := strings.TrimSpace(*source)
		if normalizedSource == "" {
			return nil, errors.New("source is required when supplied")
		}
		source = &normalizedSource
	}
	if limit < 1 || limit > MaxSimilarSourceRecordsLimit {
		return nil, fmt.Errorf("limit must be between 1 and %d", MaxSimilarSourceRecordsLimit)
	}
	return source, nil
}
