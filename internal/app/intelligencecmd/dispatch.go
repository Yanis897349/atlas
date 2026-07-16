// Package intelligencecmd parses and executes Atlas intelligence commands.
package intelligencecmd

import (
	"context"
	"io"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

// Command is one validated intelligence command.
type Command struct {
	name                 string
	eventContextQuery    intelligence.EventContextQuery
	observationIngestion ingestBLSObservationsCommand
}

// Dependencies contains the domain dependencies used by intelligence commands.
type Dependencies struct {
	Events                 intelligence.EconomicEventReader
	Observations           intelligence.ObservationReader
	ObservationPersistence intelligence.ObservationPersistence
	ObservationAdapter     intelligence.ObservationAdapter
	Embedder               search.Embedder
	SourceRecords          search.SimilarSourceRecordReader
}

// Parse recognizes and validates one intelligence command.
func Parse(arguments []string) (Command, bool, error) {
	if len(arguments) == 0 {
		return Command{}, false, nil
	}
	switch arguments[0] {
	case "economic-event-context":
		query, err := parseEconomicEventContextQuery(arguments[1:])
		if err != nil {
			return Command{}, true, err
		}
		return Command{name: arguments[0], eventContextQuery: query}, true, nil
	case "ingest-bls-observations":
		query, err := parseIngestBLSObservationsCommand(arguments[1:])
		if err != nil {
			return Command{}, true, err
		}
		return Command{name: arguments[0], observationIngestion: query}, true, nil
	default:
		return Command{}, false, nil
	}
}

// RequiresSourceRecordEmbedder reports whether the command uses semantic embeddings.
func (command Command) RequiresSourceRecordEmbedder() bool {
	return command.name == "economic-event-context"
}

// RequiresEventContextRepositories reports whether the command reads event context.
func (command Command) RequiresEventContextRepositories() bool {
	return command.name == "economic-event-context"
}

// BLSObservationEventIDs returns the canonical event bindings for BLS ingestion.
func (command Command) BLSObservationEventIDs() (string, string, bool) {
	if command.name != "ingest-bls-observations" {
		return "", "", false
	}
	return command.observationIngestion.cpiEventID,
		command.observationIngestion.employmentEventID,
		true
}

// Run executes one validated intelligence command.
func Run(
	ctx context.Context,
	dependencies Dependencies,
	stdout io.Writer,
	command Command,
) error {
	switch command.name {
	case "economic-event-context":
		return runEconomicEventContext(
			ctx,
			dependencies.Events,
			dependencies.Observations,
			dependencies.Embedder,
			dependencies.SourceRecords,
			stdout,
			command.eventContextQuery,
		)
	case "ingest-bls-observations":
		return runIngestBLSObservations(
			ctx,
			dependencies.Events,
			dependencies.ObservationAdapter,
			dependencies.ObservationPersistence,
			stdout,
			command.observationIngestion,
		)
	default:
		panic("validated intelligence command is not handled")
	}
}
