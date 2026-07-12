package search

import (
	"context"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

// SourceRecordReader retrieves canonical source records for embedding indexing.
type SourceRecordReader interface {
	RecentSourceRecords(context.Context, time.Time, time.Time, int) ([]ingestion.StoredSourceRecord, error)
}

// SourceRecordEmbeddingWriter atomically persists source-record embeddings.
type SourceRecordEmbeddingWriter interface {
	PersistSourceRecordEmbeddings(context.Context, []SourceRecordEmbedding, string) error
}

// IndexSourceRecordEmbeddings retrieves, embeds, and persists source-record titles.
func IndexSourceRecordEmbeddings(
	ctx context.Context,
	reader SourceRecordReader,
	embedder Embedder,
	writer SourceRecordEmbeddingWriter,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
	actor string,
) ([]SourceRecordEmbedding, error) {
	records, err := reader.RecentSourceRecords(ctx, windowStart, windowEnd, limit)
	if err != nil {
		return nil, fmt.Errorf("retrieve source records for embedding indexing: %w", err)
	}

	embeddings, err := EmbedSourceRecords(ctx, embedder, records)
	if err != nil {
		return nil, fmt.Errorf("embed retrieved source records for indexing: %w", err)
	}

	if err := writer.PersistSourceRecordEmbeddings(ctx, embeddings, actor); err != nil {
		return nil, fmt.Errorf("persist indexed source record embeddings: %w", err)
	}
	return embeddings, nil
}
