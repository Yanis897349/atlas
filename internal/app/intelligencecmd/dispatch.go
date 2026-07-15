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
	name  string
	query intelligence.EventContextQuery
}

// Parse recognizes and validates one intelligence command.
func Parse(arguments []string) (Command, bool, error) {
	if len(arguments) == 0 || arguments[0] != "economic-event-context" {
		return Command{}, false, nil
	}

	query, err := parseEconomicEventContextQuery(arguments[1:])
	if err != nil {
		return Command{}, true, err
	}
	return Command{name: arguments[0], query: query}, true, nil
}

// Run executes one validated intelligence command.
func Run(
	ctx context.Context,
	events intelligence.EconomicEventReader,
	observations intelligence.ObservationReader,
	embedder search.Embedder,
	sourceRecords search.SimilarSourceRecordReader,
	stdout io.Writer,
	command Command,
) error {
	switch command.name {
	case "economic-event-context":
		return runEconomicEventContext(ctx, events, observations, embedder, sourceRecords, stdout, command.query)
	default:
		panic("validated intelligence command is not handled")
	}
}
