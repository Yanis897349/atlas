package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
)

const commandUsage = "usage: atlas <migrate|ingest-rss|ingest-bls|ingest-fed|ingest-ecb|ingest-bea|ingest-eurostat|upcoming-events>"

type command struct {
	name                     string
	calendarIngestionCommand *calendarIngestionCommand
	upcomingEventsQuery      upcomingEventsQuery
}

func parseCommand(arguments []string) (command, error) {
	if len(arguments) == 0 {
		return command{}, errors.New(commandUsage)
	}

	switch arguments[0] {
	case "migrate", "ingest-rss":
		if len(arguments) != 1 {
			return command{}, errors.New(commandUsage)
		}
		return command{name: arguments[0]}, nil
	case "upcoming-events":
		query, err := parseUpcomingEventsQuery(arguments[1:])
		if err != nil {
			return command{}, err
		}
		return command{name: arguments[0], upcomingEventsQuery: query}, nil
	}

	calendarCommand := findCalendarIngestionCommand(arguments[0])
	if calendarCommand == nil {
		return command{}, fmt.Errorf("unknown command %q; %s", arguments[0], commandUsage)
	}
	if len(arguments) != 1 {
		return command{}, errors.New(commandUsage)
	}
	return command{name: arguments[0], calendarIngestionCommand: calendarCommand}, nil
}

func parseUpcomingEventsQuery(arguments []string) (upcomingEventsQuery, error) {
	const usage = "usage: atlas upcoming-events --region <united_states|eurozone> --from <RFC3339> --to <RFC3339> --limit <1-100>"

	flags := flag.NewFlagSet("upcoming-events", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var regionValue, fromValue, toValue string
	var limit int
	flags.StringVar(&regionValue, "region", "", "economic region")
	flags.StringVar(&fromValue, "from", "", "inclusive window start")
	flags.StringVar(&toValue, "to", "", "inclusive window end")
	flags.IntVar(&limit, "limit", 0, "maximum event count")
	if err := flags.Parse(arguments); err != nil {
		return upcomingEventsQuery{}, fmt.Errorf("invalid upcoming-events arguments: %w; %s", err, usage)
	}
	if flags.NArg() != 0 {
		return upcomingEventsQuery{}, fmt.Errorf("invalid upcoming-events arguments: unexpected positional arguments; %s", usage)
	}

	provided := make(map[string]bool, 4)
	flags.Visit(func(flag *flag.Flag) {
		provided[flag.Name] = true
	})
	for _, required := range []string{"region", "from", "to", "limit"} {
		if !provided[required] {
			return upcomingEventsQuery{}, fmt.Errorf("invalid upcoming-events arguments: --%s is required; %s", required, usage)
		}
	}

	regionValue = strings.TrimSpace(regionValue)
	region := calendar.Region(regionValue)
	if region != calendar.RegionUnitedStates && region != calendar.RegionEurozone {
		return upcomingEventsQuery{}, fmt.Errorf("invalid upcoming-events arguments: unsupported region %q; %s", regionValue, usage)
	}
	windowStart, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		return upcomingEventsQuery{}, fmt.Errorf("invalid upcoming-events arguments: --from must be RFC3339: %w; %s", err, usage)
	}
	windowEnd, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		return upcomingEventsQuery{}, fmt.Errorf("invalid upcoming-events arguments: --to must be RFC3339: %w; %s", err, usage)
	}
	if windowEnd.Before(windowStart) {
		return upcomingEventsQuery{}, fmt.Errorf("invalid upcoming-events arguments: --to must not be before --from; %s", usage)
	}
	if limit < 1 || limit > calendarpostgres.MaxUpcomingEventsLimit {
		return upcomingEventsQuery{}, fmt.Errorf(
			"invalid upcoming-events arguments: --limit must be between 1 and %d; %s",
			calendarpostgres.MaxUpcomingEventsLimit,
			usage,
		)
	}

	return upcomingEventsQuery{
		region:      region,
		windowStart: windowStart,
		windowEnd:   windowEnd,
		limit:       limit,
	}, nil
}
