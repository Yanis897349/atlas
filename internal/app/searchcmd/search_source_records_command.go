package searchcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/search"
)

type searchedSourceRecordOutput struct {
	ID             string  `json:"id"`
	Source         string  `json:"source"`
	SourceItemID   string  `json:"source_item_id"`
	OriginalURL    string  `json:"original_url"`
	Title          string  `json:"title"`
	PublishedAt    string  `json:"published_at"`
	RetrievedAt    string  `json:"retrieved_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	CreatedBy      string  `json:"created_by"`
	UpdatedBy      string  `json:"updated_by"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	CosineDistance float64 `json:"cosine_distance"`
}

func runSearchSourceRecords(
	ctx context.Context,
	embedder search.Embedder,
	reader search.SimilarSourceRecordReader,
	stdout io.Writer,
	command searchSourceRecordsCommand,
) error {
	results, err := search.SearchSourceRecords(ctx, embedder, reader, command.query, command.filters, command.limit)
	if err != nil {
		return fmt.Errorf("search source records: %w", err)
	}

	result := make([]searchedSourceRecordOutput, 0, len(results))
	for _, match := range results {
		record := match.SourceRecord
		result = append(result, searchedSourceRecordOutput{
			ID:             record.ID,
			Source:         record.Source,
			SourceItemID:   record.SourceItemID,
			OriginalURL:    record.OriginalURL,
			Title:          record.Title,
			PublishedAt:    output.FormatTime(record.PublishedAt),
			RetrievedAt:    output.FormatTime(record.RetrievedAt),
			CreatedAt:      output.FormatTime(record.CreatedAt),
			UpdatedAt:      output.FormatTime(record.UpdatedAt),
			CreatedBy:      record.CreatedBy,
			UpdatedBy:      record.UpdatedBy,
			Provider:       match.Provider,
			Model:          match.Model,
			CosineDistance: match.CosineDistance,
		})
	}

	var encoded bytes.Buffer
	if err := output.EncodeJSON(&encoded, "searched source records", result); err != nil {
		return err
	}
	written, err := stdout.Write(encoded.Bytes())
	if err != nil {
		return fmt.Errorf("write searched source records: %w", err)
	}
	if written != encoded.Len() {
		return fmt.Errorf("write searched source records: %w", io.ErrShortWrite)
	}
	return nil
}
