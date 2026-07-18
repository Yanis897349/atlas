package intelligencecmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
	atlasuuid "github.com/Yanis897349/atlas/internal/uuid"
)

const economicEventContextUsage = "usage: atlas economic-event-context --event-id <UUID> --from <RFC3339> --to <RFC3339> --limit <1-100> --observation-limit <1-100> --observation-revision-limit <1-100>"

func parseEconomicEventContextQuery(arguments []string) (intelligence.EventContextQuery, error) {
	flags := flag.NewFlagSet("economic-event-context", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var eventID, from, to, limitValue, observationLimitValue, observationRevisionLimitValue singleString
	flags.Var(&eventID, "event-id", "economic event UUID")
	flags.Var(&from, "from", "inclusive publication window start")
	flags.Var(&to, "to", "inclusive publication window end")
	flags.Var(&limitValue, "limit", "maximum source-record count")
	flags.Var(&observationLimitValue, "observation-limit", "maximum economic-event observation count")
	flags.Var(
		&observationRevisionLimitValue,
		"observation-revision-limit",
		"maximum revision count per economic-event observation identity",
	)
	if err := flags.Parse(arguments); err != nil {
		return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(err)
	}
	if flags.NArg() != 0 {
		return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(
			fmt.Errorf("unexpected positional arguments"),
		)
	}

	for _, required := range []struct {
		name  string
		value singleString
	}{
		{name: "event-id", value: eventID},
		{name: "from", value: from},
		{name: "to", value: to},
		{name: "limit", value: limitValue},
		{name: "observation-limit", value: observationLimitValue},
		{name: "observation-revision-limit", value: observationRevisionLimitValue},
	} {
		if !required.value.provided {
			return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(
				fmt.Errorf("--%s is required", required.name),
			)
		}
	}

	normalizedEventID, valid := atlasuuid.Normalize(eventID.value)
	if !valid {
		return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(
			fmt.Errorf("--event-id must be a UUID"),
		)
	}
	windowStart, err := parsePublicationTime("from", from.value)
	if err != nil {
		return intelligence.EventContextQuery{}, err
	}
	windowEnd, err := parsePublicationTime("to", to.value)
	if err != nil {
		return intelligence.EventContextQuery{}, err
	}
	if windowEnd.Before(windowStart) {
		return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(
			fmt.Errorf("--to must not be before --from"),
		)
	}
	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > search.MaxSimilarSourceRecordsLimit {
		return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(fmt.Errorf(
			"--limit must be between 1 and %d", search.MaxSimilarSourceRecordsLimit,
		))
	}
	observationLimit, err := strconv.Atoi(observationLimitValue.value)
	if err != nil || observationLimit < 1 || observationLimit > intelligence.MaxEventObservationsLimit {
		return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(fmt.Errorf(
			"--observation-limit must be between 1 and %d", intelligence.MaxEventObservationsLimit,
		))
	}
	observationRevisionLimit, err := strconv.Atoi(observationRevisionLimitValue.value)
	if err != nil || observationRevisionLimit < 1 || observationRevisionLimit > intelligence.MaxEventObservationsLimit {
		return intelligence.EventContextQuery{}, invalidEconomicEventContextArguments(fmt.Errorf(
			"--observation-revision-limit must be between 1 and %d",
			intelligence.MaxEventObservationsLimit,
		))
	}

	return intelligence.EventContextQuery{
		EventID:                  normalizedEventID,
		PublicationWindowStart:   windowStart.UTC(),
		PublicationWindowEnd:     windowEnd.UTC(),
		SourceRecordLimit:        limit,
		ObservationLimit:         observationLimit,
		ObservationRevisionLimit: observationRevisionLimit,
	}, nil
}

func parsePublicationTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, invalidEconomicEventContextArguments(fmt.Errorf("--%s must be RFC3339: %w", name, err))
	}
	if parsed.IsZero() {
		return time.Time{}, invalidEconomicEventContextArguments(fmt.Errorf("--%s must not be zero", name))
	}
	return parsed, nil
}

func invalidEconomicEventContextArguments(err error) error {
	return fmt.Errorf("invalid economic-event-context arguments: %w; %s", err, economicEventContextUsage)
}

type singleString struct {
	value    string
	provided bool
}

func (value *singleString) String() string {
	return value.value
}

func (value *singleString) Set(input string) error {
	if value.provided {
		return fmt.Errorf("must only be provided once")
	}
	value.value = input
	value.provided = true
	return nil
}
