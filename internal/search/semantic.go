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
	filters SimilarSourceRecordFilters,
	limit int,
) ([]SimilarSourceRecord, error) {
	filters, err := normalizeAndValidateSemanticSearchQuery(query, filters, limit)
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
		filters,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieve similar source records: %w", err)
	}
	return results, nil
}

func normalizeAndValidateSemanticSearchQuery(
	query string,
	filters SimilarSourceRecordFilters,
	limit int,
) (SimilarSourceRecordFilters, error) {
	if strings.TrimSpace(query) == "" {
		return SimilarSourceRecordFilters{}, errors.New("query is required")
	}
	filters, err := NormalizeAndValidateSimilarSourceRecordFilters(filters)
	if err != nil {
		return SimilarSourceRecordFilters{}, err
	}
	if limit < 1 || limit > MaxSimilarSourceRecordsLimit {
		return SimilarSourceRecordFilters{}, fmt.Errorf("limit must be between 1 and %d", MaxSimilarSourceRecordsLimit)
	}
	return filters, nil
}
