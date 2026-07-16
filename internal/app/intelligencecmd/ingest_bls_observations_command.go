package intelligencecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

const blsObservationIngestionActor = "atlas-bls-observation-ingestion"

func runIngestBLSObservations(
	ctx context.Context,
	adapter intelligence.ObservationAdapter,
	persistence intelligence.ObservationPersistence,
	stdout io.Writer,
	command ingestBLSObservationsCommand,
) error {
	count, err := intelligence.IngestObservations(
		ctx,
		adapter,
		persistence,
		command.limit,
		blsObservationIngestionActor,
	)
	if err != nil {
		return fmt.Errorf(
			"ingest BLS economic event observations after %d processed observations: %w",
			count,
			err,
		)
	}

	message := fmt.Sprintf("ingested %d BLS economic event observations\n", count)
	written, err := io.WriteString(stdout, message)
	if err != nil {
		return fmt.Errorf("write BLS economic event observation ingestion result: %w", err)
	}
	if written != len(message) {
		return fmt.Errorf("write BLS economic event observation ingestion result: %w", io.ErrShortWrite)
	}
	return nil
}
