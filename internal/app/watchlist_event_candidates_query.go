package app

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

const linkWatchlistEventsUsage = "usage: atlas link-watchlist-events --id <watchlist-uuid> --from <RFC3339> --to <RFC3339> --limit <1-100> --actor <actor>"

type linkWatchlistEventsCommand struct {
	watchlistID string
	windowStart time.Time
	windowEnd   time.Time
	limit       int
	actor       string
}

func parseLinkWatchlistEventsCommand(arguments []string) (linkWatchlistEventsCommand, error) {
	flags := flag.NewFlagSet("link-watchlist-events", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var id, from, to, limitValue, actor singleString
	flags.Var(&id, "id", "watchlist UUID")
	flags.Var(&from, "from", "inclusive candidate window start")
	flags.Var(&to, "to", "inclusive candidate window end")
	flags.Var(&limitValue, "limit", "maximum global candidate count")
	flags.Var(&actor, "actor", "audit actor")
	if err := flags.Parse(arguments); err != nil {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage, err,
		)
	}
	if flags.NArg() != 0 {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}

	for _, required := range []struct {
		name  string
		value *singleString
	}{{"id", &id}, {"from", &from}, {"to", &to}, {"limit", &limitValue}, {"actor", &actor}} {
		if !required.value.provided {
			return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
				"link-watchlist-events", linkWatchlistEventsUsage,
				fmt.Errorf("--%s is required", required.name),
			)
		}
	}
	if !validWatchlistID(id.value) {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage, fmt.Errorf("--id must be a UUID"),
		)
	}

	windowStart, err := time.Parse(time.RFC3339, from.value)
	if err != nil {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage,
			fmt.Errorf("--from must be RFC3339: %w", err),
		)
	}
	windowEnd, err := time.Parse(time.RFC3339, to.value)
	if err != nil {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage,
			fmt.Errorf("--to must be RFC3339: %w", err),
		)
	}
	if windowEnd.Before(windowStart) {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage,
			fmt.Errorf("--to must not be before --from"),
		)
	}

	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > calendar.MaxWatchlistEventCandidatesLimit {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage,
			fmt.Errorf("--limit must be between 1 and %d", calendar.MaxWatchlistEventCandidatesLimit),
		)
	}
	actor.value = strings.TrimSpace(actor.value)
	if actor.value == "" {
		return linkWatchlistEventsCommand{}, invalidWatchlistArguments(
			"link-watchlist-events", linkWatchlistEventsUsage, fmt.Errorf("--actor must not be blank"),
		)
	}

	return linkWatchlistEventsCommand{
		watchlistID: id.value,
		windowStart: windowStart,
		windowEnd:   windowEnd,
		limit:       limit,
		actor:       actor.value,
	}, nil
}
