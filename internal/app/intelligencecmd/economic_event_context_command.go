package intelligencecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

func runEconomicEventContext(
	ctx context.Context,
	events intelligence.EconomicEventReader,
	observations intelligence.ObservationReader,
	observationRevisions intelligence.ObservationRevisionReader,
	embedder search.Embedder,
	sourceRecords search.SimilarSourceRecordReader,
	stdout io.Writer,
	query intelligence.EventContextQuery,
) error {
	assembled, err := intelligence.AssembleEventContext(
		ctx,
		events,
		observations,
		observationRevisions,
		embedder,
		sourceRecords,
		query,
	)
	if err != nil {
		return fmt.Errorf("assemble economic event context: %w", err)
	}

	return output.EncodeJSONBuffered(
		stdout,
		"economic event context",
		newEconomicEventContextOutput(assembled),
	)
}
