package searchcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/search"
)

type indexedSourceRecordOutput struct {
	SourceRecordID string `json:"source_record_id"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Dimension      int    `json:"dimension"`
}

func runIndexSourceRecords(
	ctx context.Context,
	reader search.SourceRecordReader,
	embedder search.Embedder,
	writer search.SourceRecordEmbeddingWriter,
	stdout io.Writer,
	command indexSourceRecordsCommand,
) error {
	embeddings, err := search.IndexSourceRecordEmbeddings(
		ctx,
		reader,
		embedder,
		writer,
		command.windowStart,
		command.windowEnd,
		command.limit,
		command.actor,
	)
	if err != nil {
		return fmt.Errorf("index source records: %w", err)
	}

	result := make([]indexedSourceRecordOutput, 0, len(embeddings))
	for _, embedding := range embeddings {
		result = append(result, indexedSourceRecordOutput{
			SourceRecordID: embedding.SourceRecordID,
			Provider:       embedding.Provider,
			Model:          embedding.Model,
			Dimension:      len(embedding.Vector),
		})
	}

	return output.EncodeJSONBuffered(stdout, "indexed source records", result)
}
