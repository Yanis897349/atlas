package intelligencecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/intelligence"
)

func runObservationRevisions(
	ctx context.Context,
	revisions intelligence.ObservationRevisionReader,
	stdout io.Writer,
	query observationRevisionsQuery,
) error {
	stored, err := revisions.ObservationRevisions(
		ctx,
		query.eventID,
		query.source,
		query.sourceObservationID,
		query.limit,
	)
	if err != nil {
		return fmt.Errorf("retrieve economic event observation revisions: %w", err)
	}

	result := make([]economicEventObservationOutput, 0, len(stored))
	for _, revision := range stored {
		result = append(result, newEconomicEventObservationOutput(revision))
	}

	return output.EncodeJSONBuffered(stdout, "economic event observation revisions", result)
}
