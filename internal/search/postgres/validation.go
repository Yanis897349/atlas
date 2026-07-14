package postgres

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Yanis897349/atlas/internal/search"
	"github.com/Yanis897349/atlas/internal/search/vector"
	"github.com/jackc/pgx/v5/pgtype"
)

type embeddingReference struct {
	sourceRecordID string
	provider       string
	model          string
}

func normalizeAndValidateEmbeddings(
	embeddings []search.SourceRecordEmbedding,
	actor string,
) ([]search.SourceRecordEmbedding, string, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, "", errors.New("actor is required")
	}

	normalized := make([]search.SourceRecordEmbedding, len(embeddings))
	seen := make(map[embeddingReference]struct{}, len(embeddings))
	dimension := 0
	for index, embedding := range embeddings {
		sourceRecordID, valid := normalizeUUID(embedding.SourceRecordID)
		if !valid {
			return nil, "", fmt.Errorf("embedding %d source record ID must be a UUID", index)
		}
		embedding.SourceRecordID = sourceRecordID
		embedding.Provider = strings.TrimSpace(embedding.Provider)
		if embedding.Provider == "" {
			return nil, "", fmt.Errorf("embedding %d provider is required", index)
		}
		embedding.Model = strings.TrimSpace(embedding.Model)
		if embedding.Model == "" {
			return nil, "", fmt.Errorf("embedding %d model is required", index)
		}
		if err := vector.Validate(embedding.Vector); err != nil {
			return nil, "", fmt.Errorf("embedding %d %w", index, err)
		}
		if index == 0 {
			dimension = len(embedding.Vector)
		} else if len(embedding.Vector) != dimension {
			return nil, "", fmt.Errorf(
				"embedding %d vector dimension %d does not match batch dimension %d",
				index,
				len(embedding.Vector),
				dimension,
			)
		}
		reference := embeddingReference{
			sourceRecordID: embedding.SourceRecordID,
			provider:       embedding.Provider,
			model:          embedding.Model,
		}
		if _, exists := seen[reference]; exists {
			return nil, "", fmt.Errorf(
				"embedding %d duplicates source record %q provider %q model %q",
				index,
				embedding.SourceRecordID,
				embedding.Provider,
				embedding.Model,
			)
		}
		seen[reference] = struct{}{}

		embedding.Vector = append([]float32(nil), embedding.Vector...)
		normalized[index] = embedding
	}
	return normalized, actor, nil
}

func normalizeAndValidateSimilarityQuery(
	provider string,
	model string,
	queryVector []float32,
	source *string,
	limit int,
) (string, string, *string, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "", "", nil, errors.New("provider is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", "", nil, errors.New("model is required")
	}
	if err := vector.Validate(queryVector); err != nil {
		return "", "", nil, fmt.Errorf("query %w", err)
	}
	if source != nil {
		normalizedSource := strings.TrimSpace(*source)
		if normalizedSource == "" {
			return "", "", nil, errors.New("source is required when supplied")
		}
		source = &normalizedSource
	}
	if limit < 1 || limit > search.MaxSimilarSourceRecordsLimit {
		return "", "", nil, fmt.Errorf("limit must be between 1 and %d", search.MaxSimilarSourceRecordsLimit)
	}
	return provider, model, source, nil
}

func normalizeUUID(value string) (string, bool) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return "", false
	}
	return id.String(), true
}
