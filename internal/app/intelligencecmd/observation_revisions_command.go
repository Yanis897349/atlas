package intelligencecmd

import (
	"bytes"
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

	var encoded bytes.Buffer
	if err := output.EncodeJSON(&encoded, "economic event observation revisions", result); err != nil {
		return err
	}
	written, err := stdout.Write(encoded.Bytes())
	if err != nil {
		return fmt.Errorf("write economic event observation revisions: %w", err)
	}
	if written != encoded.Len() {
		return fmt.Errorf("write economic event observation revisions: %w", io.ErrShortWrite)
	}
	return nil
}
