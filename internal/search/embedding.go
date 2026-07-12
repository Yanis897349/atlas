package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/search/vector"
)

// EmbedSourceRecords embeds persisted source-record titles in input order.
func EmbedSourceRecords(
	ctx context.Context,
	embedder Embedder,
	records []ingestion.StoredSourceRecord,
) ([]SourceRecordEmbedding, error) {
	inputs, err := embeddingInputs(records)
	if err != nil {
		return nil, fmt.Errorf("validate source record embedding input: %w", err)
	}
	if len(inputs) == 0 {
		return []SourceRecordEmbedding{}, nil
	}

	batch, err := embedder.Embed(ctx, inputs)
	if err != nil {
		return nil, fmt.Errorf("embed source records with provider: %w", err)
	}

	embeddings, err := validateEmbeddingBatch(inputs, batch)
	if err != nil {
		return nil, fmt.Errorf("validate source record embeddings: %w", err)
	}
	return embeddings, nil
}

func embeddingInputs(records []ingestion.StoredSourceRecord) ([]EmbeddingInput, error) {
	inputs := make([]EmbeddingInput, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for index, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			return nil, fmt.Errorf("source record %d ID is required", index)
		}
		if strings.TrimSpace(record.Title) == "" {
			return nil, fmt.Errorf("source record %d title is required", index)
		}
		if _, exists := seen[record.ID]; exists {
			return nil, fmt.Errorf("source record %d ID %q is duplicated", index, record.ID)
		}
		seen[record.ID] = struct{}{}
		inputs = append(inputs, EmbeddingInput{SourceRecordID: record.ID, Text: record.Title})
	}
	return inputs, nil
}

func validateEmbeddingBatch(inputs []EmbeddingInput, batch EmbeddingBatch) ([]SourceRecordEmbedding, error) {
	provider := strings.TrimSpace(batch.Provider)
	if provider == "" {
		return nil, errors.New("provider is required")
	}
	model := strings.TrimSpace(batch.Model)
	if model == "" {
		return nil, errors.New("model is required")
	}
	if len(batch.Embeddings) != len(inputs) {
		return nil, fmt.Errorf(
			"provider returned %d embeddings for %d source records",
			len(batch.Embeddings),
			len(inputs),
		)
	}

	dimension := 0
	embeddings := make([]SourceRecordEmbedding, 0, len(inputs))
	for index, providerEmbedding := range batch.Embeddings {
		if providerEmbedding.SourceRecordID != inputs[index].SourceRecordID {
			return nil, fmt.Errorf(
				"embedding %d source record ID %q does not match input ID %q",
				index,
				providerEmbedding.SourceRecordID,
				inputs[index].SourceRecordID,
			)
		}
		if err := vector.Validate(providerEmbedding.Vector); err != nil {
			return nil, fmt.Errorf("embedding %d %w", index, err)
		}
		if index == 0 {
			dimension = len(providerEmbedding.Vector)
		} else if len(providerEmbedding.Vector) != dimension {
			return nil, fmt.Errorf(
				"embedding %d vector dimension %d does not match batch dimension %d",
				index,
				len(providerEmbedding.Vector),
				dimension,
			)
		}
		embeddings = append(embeddings, SourceRecordEmbedding{
			SourceRecordID: providerEmbedding.SourceRecordID,
			Provider:       provider,
			Model:          model,
			Vector:         append([]float32(nil), providerEmbedding.Vector...),
		})
	}
	return embeddings, nil
}
