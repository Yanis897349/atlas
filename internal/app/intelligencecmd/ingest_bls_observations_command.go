package intelligencecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarbls "github.com/Yanis897349/atlas/internal/calendar/bls"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencebls "github.com/Yanis897349/atlas/internal/intelligence/bls"
)

const (
	blsObservationIngestionActor = "atlas-bls-observation-ingestion"
	blsCPIEventName              = "Consumer Price Index"
	blsEmploymentEventName       = "Employment Situation"
)

type blsObservationBinding struct {
	label           string
	economicEventID string
	source          string
	region          calendar.Region
	name            string
	eventType       calendar.EventType
	series          intelligencebls.Series
}

func runIngestBLSObservations(
	ctx context.Context,
	events intelligence.EconomicEventReader,
	adapter intelligence.ObservationAdapter,
	writer intelligence.ObservationWriter,
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
		writer,
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
	for _, binding := range command.blsObservationBindings() {
		event, err := events.EconomicEvent(ctx, binding.economicEventID)
		if err != nil {
			return fmt.Errorf("retrieve %s economic event: %w", binding.label, err)
		}
		if event.Source != binding.source || event.Region != binding.region ||
			event.Name != binding.name || event.Type != binding.eventType {
			return fmt.Errorf(
				"%s economic event %q must be the canonical BLS %s release",
				binding.label,
				binding.economicEventID,
				binding.name,
			)
		}
	}
	return nil
}

func (command ingestBLSObservationsCommand) blsObservationBindings() []blsObservationBinding {
	return []blsObservationBinding{
		{
			label:           "CPI",
			economicEventID: command.cpiEventID,
			source:          calendarbls.Source,
			region:          calendar.RegionUnitedStates,
			name:            blsCPIEventName,
			eventType:       calendar.EventTypeInflation,
			series:          intelligencebls.SeriesCPIAllItemsNSA,
		},
		{
			label:           "Employment Situation",
			economicEventID: command.employmentEventID,
			source:          calendarbls.Source,
			region:          calendar.RegionUnitedStates,
			name:            blsEmploymentEventName,
			eventType:       calendar.EventTypeEmployment,
			series:          intelligencebls.SeriesTotalNonfarmPayrollSA,
		},
	}
}

func (command ingestBLSObservationsCommand) blsObservationTargets() []intelligencebls.Target {
	bindings := command.blsObservationBindings()
	targets := make([]intelligencebls.Target, 0, len(bindings))
	for _, binding := range bindings {
		targets = append(targets, intelligencebls.Target{
			EconomicEventID: binding.economicEventID,
			Series:          binding.series,
		})
	}
	return targets
}
