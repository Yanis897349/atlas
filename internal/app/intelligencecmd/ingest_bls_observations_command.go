package intelligencecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarbls "github.com/Yanis897349/atlas/internal/calendar/bls"
	"github.com/Yanis897349/atlas/internal/intelligence"
)

const (
	blsObservationIngestionActor = "atlas-bls-observation-ingestion"
	blsCPIEventName              = "Consumer Price Index"
	blsEmploymentEventName       = "Employment Situation"
)

func runIngestBLSObservations(
	ctx context.Context,
	events intelligence.EconomicEventReader,
	adapter intelligence.ObservationAdapter,
	persistence intelligence.ObservationPersistence,
	stdout io.Writer,
	command ingestBLSObservationsCommand,
) error {
	if err := validateBLSObservationEvents(ctx, events, command); err != nil {
		return fmt.Errorf(
			"ingest BLS economic event observations after 0 processed observations: %w",
			err,
		)
	}

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

func validateBLSObservationEvents(
	ctx context.Context,
	events intelligence.EconomicEventReader,
	command ingestBLSObservationsCommand,
) error {
	expected := []struct {
		label     string
		id        string
		name      string
		eventType calendar.EventType
	}{
		{label: "CPI", id: command.cpiEventID, name: blsCPIEventName, eventType: calendar.EventTypeInflation},
		{label: "Employment Situation", id: command.employmentEventID, name: blsEmploymentEventName, eventType: calendar.EventTypeEmployment},
	}
	for _, expectation := range expected {
		event, err := events.EconomicEvent(ctx, expectation.id)
		if err != nil {
			return fmt.Errorf("retrieve %s economic event: %w", expectation.label, err)
		}
		if event.Source != calendarbls.Source || event.Region != calendar.RegionUnitedStates ||
			event.Name != expectation.name || event.Type != expectation.eventType {
			return fmt.Errorf(
				"%s economic event %q must be the canonical BLS %s release",
				expectation.label,
				expectation.id,
				expectation.name,
			)
		}
	}
	return nil
}
