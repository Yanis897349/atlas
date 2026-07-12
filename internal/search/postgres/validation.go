package postgres

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Yanis897349/atlas/internal/search"
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
		if len(embedding.Vector) == 0 {
			return nil, "", fmt.Errorf("embedding %d vector is required", index)
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
		for valueIndex, value := range embedding.Vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, "", fmt.Errorf("embedding %d vector value %d must be finite", index, valueIndex)
			}
		}
		if !hasNonZeroVectorNorm(embedding.Vector) {
			return nil, "", fmt.Errorf("embedding %d vector must have non-zero norm", index)
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
	limit int,
) (string, string, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "", "", errors.New("provider is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", "", errors.New("model is required")
	}
	if len(queryVector) == 0 {
		return "", "", errors.New("query vector is required")
	}
	for index, value := range queryVector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", "", fmt.Errorf("query vector value %d must be finite", index)
		}
	}
	if !hasNonZeroVectorNorm(queryVector) {
		return "", "", errors.New("query vector must have non-zero norm")
	}
	if limit < 1 || limit > search.MaxSimilarSourceRecordsLimit {
		return "", "", fmt.Errorf("limit must be between 1 and %d", search.MaxSimilarSourceRecordsLimit)
	}
	return provider, model, nil
}

func hasNonZeroVectorNorm(vector []float32) bool {
	var squaredNorm float64
	for _, value := range vector {
		squaredNorm += float64(value) * float64(value)
	}
	return squaredNorm > 0
}

func normalizeUUID(value string) (string, bool) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return "", false
	}
	return id.String(), true
}
