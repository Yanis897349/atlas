package app

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

const dailyBriefInputUsage = "usage: atlas daily-brief-input --region <united_states|eurozone> --publication-from <RFC3339> --publication-to <RFC3339> --source-record-limit <1-100> --event-from <RFC3339> --event-to <RFC3339> --upcoming-event-limit <1-100>"

func parseDailyBriefInputQuery(arguments []string) (dailyBriefInputQuery, error) {
	flags := flag.NewFlagSet("daily-brief-input", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var regionValue, publicationFrom, publicationTo, eventFrom, eventTo string
	var sourceRecordLimit, upcomingEventLimit int
	flags.StringVar(&regionValue, "region", "", "economic region")
	flags.StringVar(&publicationFrom, "publication-from", "", "inclusive publication window start")
	flags.StringVar(&publicationTo, "publication-to", "", "inclusive publication window end")
	flags.IntVar(&sourceRecordLimit, "source-record-limit", 0, "maximum source record count")
	flags.StringVar(&eventFrom, "event-from", "", "inclusive event window start")
	flags.StringVar(&eventTo, "event-to", "", "inclusive event window end")
	flags.IntVar(&upcomingEventLimit, "upcoming-event-limit", 0, "maximum upcoming event count")
	if err := flags.Parse(arguments); err != nil {
		return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(err)
	}
	if flags.NArg() != 0 {
		return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(fmt.Errorf("unexpected positional arguments"))
	}

	provided := make(map[string]bool, 7)
	flags.Visit(func(flag *flag.Flag) {
		provided[flag.Name] = true
	})
	for _, required := range []string{
		"region",
		"publication-from",
		"publication-to",
		"source-record-limit",
		"event-from",
		"event-to",
		"upcoming-event-limit",
	} {
		if !provided[required] {
			return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(fmt.Errorf("--%s is required", required))
		}
	}

	regionValue = strings.TrimSpace(regionValue)
	region := calendar.Region(regionValue)
	if region != calendar.RegionUnitedStates && region != calendar.RegionEurozone {
		return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(fmt.Errorf("unsupported region %q", regionValue))
	}
	publicationWindowStart, err := parseDailyBriefTime("--publication-from", publicationFrom)
	if err != nil {
		return dailyBriefInputQuery{}, err
	}
	publicationWindowEnd, err := parseDailyBriefTime("--publication-to", publicationTo)
	if err != nil {
		return dailyBriefInputQuery{}, err
	}
	if publicationWindowEnd.Before(publicationWindowStart) {
		return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(fmt.Errorf("--publication-to must not be before --publication-from"))
	}
	if sourceRecordLimit < 1 || sourceRecordLimit > ingestionpostgres.MaxRecentSourceRecordsLimit {
		return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(fmt.Errorf(
			"--source-record-limit must be between 1 and %d",
			ingestionpostgres.MaxRecentSourceRecordsLimit,
		))
	}

	eventWindowStart, err := parseDailyBriefTime("--event-from", eventFrom)
	if err != nil {
		return dailyBriefInputQuery{}, err
	}
	eventWindowEnd, err := parseDailyBriefTime("--event-to", eventTo)
	if err != nil {
		return dailyBriefInputQuery{}, err
	}
	if eventWindowEnd.Before(eventWindowStart) {
		return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(fmt.Errorf("--event-to must not be before --event-from"))
	}
	if upcomingEventLimit < 1 || upcomingEventLimit > calendarpostgres.MaxUpcomingEventsLimit {
		return dailyBriefInputQuery{}, invalidDailyBriefInputArguments(fmt.Errorf(
			"--upcoming-event-limit must be between 1 and %d",
			calendarpostgres.MaxUpcomingEventsLimit,
		))
	}

	return dailyBriefInputQuery{
		region:                 region,
		publicationWindowStart: publicationWindowStart,
		publicationWindowEnd:   publicationWindowEnd,
		sourceRecordLimit:      sourceRecordLimit,
		eventWindowStart:       eventWindowStart,
		eventWindowEnd:         eventWindowEnd,
		upcomingEventLimit:     upcomingEventLimit,
	}, nil
}

func parseDailyBriefTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, invalidDailyBriefInputArguments(fmt.Errorf("%s must be RFC3339: %w", name, err))
	}
	return parsed, nil
}

func invalidDailyBriefInputArguments(err error) error {
	return fmt.Errorf("invalid daily-brief-input arguments: %w; %s", err, dailyBriefInputUsage)
}
