package intelligencecmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/Yanis897349/atlas/internal/intelligence"
	atlasuuid "github.com/Yanis897349/atlas/internal/uuid"
)

const ingestBLSObservationsUsage = "usage: atlas ingest-bls-observations --cpi-event-id <UUID> --employment-event-id <UUID> --limit <1-100>"

type ingestBLSObservationsCommand struct {
	cpiEventID        string
	employmentEventID string
	limit             int
}

func parseIngestBLSObservationsCommand(arguments []string) (ingestBLSObservationsCommand, error) {
	flags := flag.NewFlagSet("ingest-bls-observations", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var cpiEventID, employmentEventID, limitValue singleString
	flags.Var(&cpiEventID, "cpi-event-id", "canonical CPI economic event UUID")
	flags.Var(&employmentEventID, "employment-event-id", "canonical Employment Situation event UUID")
	flags.Var(&limitValue, "limit", "maximum observation count")
	if err := flags.Parse(arguments); err != nil {
		return ingestBLSObservationsCommand{}, invalidIngestBLSObservationsArguments(err)
	}
	if flags.NArg() != 0 {
		return ingestBLSObservationsCommand{}, invalidIngestBLSObservationsArguments(
			fmt.Errorf("unexpected positional arguments"),
		)
	}

	for _, required := range []struct {
		name  string
		value singleString
	}{
		{name: "cpi-event-id", value: cpiEventID},
		{name: "employment-event-id", value: employmentEventID},
		{name: "limit", value: limitValue},
	} {
		if !required.value.provided {
			return ingestBLSObservationsCommand{}, invalidIngestBLSObservationsArguments(
				fmt.Errorf("--%s is required", required.name),
			)
		}
	}

	normalizedCPIEventID, valid := atlasuuid.Normalize(cpiEventID.value)
	if !valid {
		return ingestBLSObservationsCommand{}, invalidIngestBLSObservationsArguments(
			fmt.Errorf("--cpi-event-id must be a UUID"),
		)
	}
	normalizedEmploymentEventID, valid := atlasuuid.Normalize(employmentEventID.value)
	if !valid {
		return ingestBLSObservationsCommand{}, invalidIngestBLSObservationsArguments(
			fmt.Errorf("--employment-event-id must be a UUID"),
		)
	}
	if normalizedCPIEventID == normalizedEmploymentEventID {
		return ingestBLSObservationsCommand{}, invalidIngestBLSObservationsArguments(
			fmt.Errorf("--employment-event-id must differ from --cpi-event-id"),
		)
	}

	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > intelligence.MaxObservationIngestionLimit {
		return ingestBLSObservationsCommand{}, invalidIngestBLSObservationsArguments(fmt.Errorf(
			"--limit must be between 1 and %d", intelligence.MaxObservationIngestionLimit,
		))
	}

	return ingestBLSObservationsCommand{
		cpiEventID:        normalizedCPIEventID,
		employmentEventID: normalizedEmploymentEventID,
		limit:             limit,
	}, nil
}

func invalidIngestBLSObservationsArguments(err error) error {
	return fmt.Errorf("invalid ingest-bls-observations arguments: %w; %s", err, ingestBLSObservationsUsage)
}
