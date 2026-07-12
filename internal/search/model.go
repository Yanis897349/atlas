// Package search defines source-record embedding boundaries.
package search

import "context"

// EmbeddingInput is one source record prepared for a provider.
type EmbeddingInput struct {
	SourceRecordID string
	Text           string
}

// ProviderEmbedding is one provider vector associated with its source record.
type ProviderEmbedding struct {
	SourceRecordID string
	Vector         []float32
}

// EmbeddingBatch contains provider vectors and their model provenance.
type EmbeddingBatch struct {
	Provider   string
	Model      string
	Embeddings []ProviderEmbedding
}

// SourceRecordEmbedding is a validated source-record vector with model provenance.
type SourceRecordEmbedding struct {
	SourceRecordID string
	Provider       string
	Model          string
	Vector         []float32
}

// Embedder produces vectors for an ordered source-record input batch.
type Embedder interface {
	Embed(context.Context, []EmbeddingInput) (EmbeddingBatch, error)
}
