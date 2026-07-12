package watchlistcmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

const (
	linkWatchlistEventUsage = "usage: atlas link-watchlist-event --id <watchlist-uuid> --symbol <symbol> --event-id <event-uuid> --actor <actor>"
	watchlistEventsUsage    = "usage: atlas watchlist-events --id <watchlist-uuid> --symbol <symbol> --limit <1-100>"
)

type linkWatchlistEventCommand struct {
	watchlistID string
	symbol      string
	eventID     string
	actor       string
}

type watchlistEventsQuery struct {
	watchlistID string
	symbol      string
	limit       int
}

func parseLinkWatchlistEventCommand(arguments []string) (linkWatchlistEventCommand, error) {
	flags := flag.NewFlagSet("link-watchlist-event", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var id, symbol, eventID, actor singleString
	flags.Var(&id, "id", "watchlist UUID")
	flags.Var(&symbol, "symbol", "instrument symbol")
	flags.Var(&eventID, "event-id", "economic event UUID")
	flags.Var(&actor, "actor", "audit actor")
	if err := flags.Parse(arguments); err != nil {
		return linkWatchlistEventCommand{}, invalidWatchlistArguments("link-watchlist-event", linkWatchlistEventUsage, err)
	}
	if flags.NArg() != 0 {
		return linkWatchlistEventCommand{}, invalidWatchlistArguments(
			"link-watchlist-event", linkWatchlistEventUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}
	for _, required := range []struct {
		name  string
		value *singleString
	}{{"id", &id}, {"symbol", &symbol}, {"event-id", &eventID}, {"actor", &actor}} {
		if !required.value.provided {
			return linkWatchlistEventCommand{}, invalidWatchlistArguments(
				"link-watchlist-event", linkWatchlistEventUsage, fmt.Errorf("--%s is required", required.name),
			)
		}
	}
	if !validWatchlistID(id.value) {
		return linkWatchlistEventCommand{}, invalidWatchlistArguments(
			"link-watchlist-event", linkWatchlistEventUsage, fmt.Errorf("--id must be a UUID"),
		)
	}
	if !validWatchlistID(eventID.value) {
		return linkWatchlistEventCommand{}, invalidWatchlistArguments(
			"link-watchlist-event", linkWatchlistEventUsage, fmt.Errorf("--event-id must be a UUID"),
		)
	}
	symbol.value = watchlist.NormalizeInstrumentSymbol(symbol.value)
	actor.value = strings.TrimSpace(actor.value)
	if symbol.value == "" {
		return linkWatchlistEventCommand{}, invalidWatchlistArguments(
			"link-watchlist-event", linkWatchlistEventUsage, fmt.Errorf("--symbol must not be blank"),
		)
	}
	if actor.value == "" {
		return linkWatchlistEventCommand{}, invalidWatchlistArguments(
			"link-watchlist-event", linkWatchlistEventUsage, fmt.Errorf("--actor must not be blank"),
		)
	}
	return linkWatchlistEventCommand{
		watchlistID: id.value, symbol: symbol.value, eventID: eventID.value, actor: actor.value,
	}, nil
}

func parseWatchlistEventsQuery(arguments []string) (watchlistEventsQuery, error) {
	flags := flag.NewFlagSet("watchlist-events", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var id, symbol, limitValue singleString
	flags.Var(&id, "id", "watchlist UUID")
	flags.Var(&symbol, "symbol", "instrument symbol")
	flags.Var(&limitValue, "limit", "maximum linked event count")
	if err := flags.Parse(arguments); err != nil {
		return watchlistEventsQuery{}, invalidWatchlistArguments("watchlist-events", watchlistEventsUsage, err)
	}
	if flags.NArg() != 0 {
		return watchlistEventsQuery{}, invalidWatchlistArguments(
			"watchlist-events", watchlistEventsUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}
	for _, required := range []struct {
		name  string
		value *singleString
	}{{"id", &id}, {"symbol", &symbol}, {"limit", &limitValue}} {
		if !required.value.provided {
			return watchlistEventsQuery{}, invalidWatchlistArguments(
				"watchlist-events", watchlistEventsUsage, fmt.Errorf("--%s is required", required.name),
			)
		}
	}
	if !validWatchlistID(id.value) {
		return watchlistEventsQuery{}, invalidWatchlistArguments(
			"watchlist-events", watchlistEventsUsage, fmt.Errorf("--id must be a UUID"),
		)
	}
	symbol.value = watchlist.NormalizeInstrumentSymbol(symbol.value)
	if symbol.value == "" {
		return watchlistEventsQuery{}, invalidWatchlistArguments(
			"watchlist-events", watchlistEventsUsage, fmt.Errorf("--symbol must not be blank"),
		)
	}
	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > watchlist.MaxEventLinksLimit {
		return watchlistEventsQuery{}, invalidWatchlistArguments(
			"watchlist-events", watchlistEventsUsage,
			fmt.Errorf("--limit must be between 1 and %d", watchlist.MaxEventLinksLimit),
		)
	}
	return watchlistEventsQuery{watchlistID: id.value, symbol: symbol.value, limit: limit}, nil
}
