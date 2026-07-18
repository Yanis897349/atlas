package intelligencecmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Yanis897349/atlas/internal/intelligence"
	atlasuuid "github.com/Yanis897349/atlas/internal/uuid"
)

const observationRevisionsUsage = "usage: atlas economic-event-observation-revisions --event-id <UUID> --source <source> --source-observation-id <identity> --limit <1-100>"

type observationRevisionsQuery struct {
	eventID             string
	source              string
	sourceObservationID string
	limit               int
}

func parseObservationRevisionsQuery(arguments []string) (observationRevisionsQuery, error) {
	flags := flag.NewFlagSet("economic-event-observation-revisions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var eventID, source, sourceObservationID, limitValue singleString
	flags.Var(&eventID, "event-id", "economic event UUID")
	flags.Var(&source, "source", "exact observation source")
	flags.Var(&sourceObservationID, "source-observation-id", "exact source observation identity")
	flags.Var(&limitValue, "limit", "maximum revision count")
	if err := flags.Parse(arguments); err != nil {
		return observationRevisionsQuery{}, invalidObservationRevisionsArguments(err)
	}
	if flags.NArg() != 0 {
		return observationRevisionsQuery{}, invalidObservationRevisionsArguments(
			fmt.Errorf("unexpected positional arguments"),
		)
	}

	for _, required := range []struct {
		name  string
		value singleString
	}{
		{name: "event-id", value: eventID},
		{name: "source", value: source},
		{name: "source-observation-id", value: sourceObservationID},
		{name: "limit", value: limitValue},
	} {
		if !required.value.provided {
			return observationRevisionsQuery{}, invalidObservationRevisionsArguments(
				fmt.Errorf("--%s is required", required.name),
			)
		}
	}

	normalizedEventID, valid := atlasuuid.Normalize(eventID.value)
	if !valid {
		return observationRevisionsQuery{}, invalidObservationRevisionsArguments(
			fmt.Errorf("--event-id must be a UUID"),
		)
	}
	normalizedSource := strings.TrimSpace(source.value)
	if normalizedSource == "" {
		return observationRevisionsQuery{}, invalidObservationRevisionsArguments(
			fmt.Errorf("--source must not be blank"),
		)
	}
	normalizedSourceObservationID := strings.TrimSpace(sourceObservationID.value)
	if normalizedSourceObservationID == "" {
		return observationRevisionsQuery{}, invalidObservationRevisionsArguments(
			fmt.Errorf("--source-observation-id must not be blank"),
		)
	}
	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > intelligence.MaxEventObservationsLimit {
		return observationRevisionsQuery{}, invalidObservationRevisionsArguments(fmt.Errorf(
			"--limit must be between 1 and %d", intelligence.MaxEventObservationsLimit,
		))
	}

	return observationRevisionsQuery{
		eventID:             normalizedEventID,
		source:              normalizedSource,
		sourceObservationID: normalizedSourceObservationID,
		limit:               limit,
	}, nil
}

func invalidObservationRevisionsArguments(err error) error {
	return fmt.Errorf(
		"invalid economic-event-observation-revisions arguments: %w; %s",
		err,
		observationRevisionsUsage,
	)
}
