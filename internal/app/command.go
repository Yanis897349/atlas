package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/app/searchcmd"
	"github.com/Yanis897349/atlas/internal/app/watchlistcmd"
	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
)

const commandUsage = "usage: atlas <migrate|ingest-rss|ingest-bls|ingest-fed|ingest-ecb|ingest-bea|ingest-census|ingest-eurostat|ingest-spglobal|upcoming-events|daily-brief-input|daily-brief|daily-briefs|create-watchlist|update-watchlist|delete-watchlist|watchlist|watchlists|link-watchlist-event|link-watchlist-events|unlink-watchlist-event|watchlist-events|index-source-records|search-source-records>"

type command struct {
	name                     string
	calendarIngestionCommand *calendarIngestionCommand
	upcomingEventsQuery      upcomingEventsQuery
	dailyBriefInputQuery     dailybrief.InputQuery
	storedDailyBriefsQuery   storedDailyBriefsQuery
	watchlistCommand         *watchlistcmd.Command
	searchCommand            *searchcmd.Command
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
	case "daily-brief-input":
		query, err := parseDailyBriefInputQuery(arguments[1:])
		if err != nil {
			return command{}, err
		}
		return command{name: arguments[0], dailyBriefInputQuery: query}, nil
	case "daily-brief":
		query, err := parseDailyBriefQuery(arguments[0], arguments[1:])
		if err != nil {
			return command{}, err
		}
		return command{name: arguments[0], dailyBriefInputQuery: query}, nil
	case "daily-briefs":
		query, err := parseStoredDailyBriefsQuery(arguments[1:])
		if err != nil {
			return command{}, err
		}
		return command{name: arguments[0], storedDailyBriefsQuery: query}, nil
	}
	searchCommand, recognized, err := searchcmd.Parse(arguments)
	if err != nil {
		return command{}, err
	}
	if recognized {
		return command{name: arguments[0], searchCommand: &searchCommand}, nil
	}

	watchlistCommand, recognized, err := watchlistcmd.Parse(arguments)
	if err != nil {
		return command{}, err
	}
	if recognized {
		return command{name: arguments[0], watchlistCommand: &watchlistCommand}, nil
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

func parseStoredDailyBriefsQuery(arguments []string) (storedDailyBriefsQuery, error) {
	return parseRegionWindowQuery("daily-briefs", dailybrief.MaxStoredBriefsLimit, arguments)
}

func parseUpcomingEventsQuery(arguments []string) (upcomingEventsQuery, error) {
	return parseRegionWindowQuery("upcoming-events", calendar.MaxUpcomingEventsLimit, arguments)
}

type regionWindowQuery struct {
	region      calendar.Region
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func parseRegionWindowQuery(commandName string, maxLimit int, arguments []string) (regionWindowQuery, error) {
	usage := fmt.Sprintf(
		"usage: atlas %s --region <united_states|eurozone> --from <RFC3339> --to <RFC3339> --limit <1-%d>",
		commandName,
		maxLimit,
	)

	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var regionValue, fromValue, toValue string
	var limit int
	flags.StringVar(&regionValue, "region", "", "economic region")
	flags.StringVar(&fromValue, "from", "", "inclusive window start")
	flags.StringVar(&toValue, "to", "", "inclusive window end")
	flags.IntVar(&limit, "limit", 0, "maximum result count")
	if err := flags.Parse(arguments); err != nil {
		return regionWindowQuery{}, fmt.Errorf("invalid %s arguments: %w; %s", commandName, err, usage)
	}
	if flags.NArg() != 0 {
		return regionWindowQuery{}, fmt.Errorf("invalid %s arguments: unexpected positional arguments; %s", commandName, usage)
	}

	provided := make(map[string]bool, 4)
	flags.Visit(func(flag *flag.Flag) {
		provided[flag.Name] = true
	})
	for _, required := range []string{"region", "from", "to", "limit"} {
		if !provided[required] {
			return regionWindowQuery{}, fmt.Errorf("invalid %s arguments: --%s is required; %s", commandName, required, usage)
		}
	}

	regionValue = strings.TrimSpace(regionValue)
	region := calendar.Region(regionValue)
	if region != calendar.RegionUnitedStates && region != calendar.RegionEurozone {
		return regionWindowQuery{}, fmt.Errorf("invalid %s arguments: unsupported region %q; %s", commandName, regionValue, usage)
	}
	windowStart, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		return regionWindowQuery{}, fmt.Errorf("invalid %s arguments: --from must be RFC3339: %w; %s", commandName, err, usage)
	}
	windowEnd, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		return regionWindowQuery{}, fmt.Errorf("invalid %s arguments: --to must be RFC3339: %w; %s", commandName, err, usage)
	}
	if windowEnd.Before(windowStart) {
		return regionWindowQuery{}, fmt.Errorf("invalid %s arguments: --to must not be before --from; %s", commandName, usage)
	}
	if limit < 1 || limit > maxLimit {
		return regionWindowQuery{}, fmt.Errorf(
			"invalid %s arguments: --limit must be between 1 and %d; %s",
			commandName,
			maxLimit,
			usage,
		)
	}

	return regionWindowQuery{
		region:      region,
		windowStart: windowStart,
		windowEnd:   windowEnd,
		limit:       limit,
	}, nil
}
