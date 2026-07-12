package watchlistcmd

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

const unlinkWatchlistEventUsage = "usage: atlas unlink-watchlist-event --id <watchlist-uuid> --symbol <symbol> --event-id <event-uuid>"

type unlinkWatchlistEventCommand struct {
	watchlistID string
	symbol      string
	eventID     string
}

func parseUnlinkWatchlistEventCommand(arguments []string) (unlinkWatchlistEventCommand, error) {
	flags := flag.NewFlagSet("unlink-watchlist-event", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var id, symbol, eventID singleString
	flags.Var(&id, "id", "watchlist UUID")
	flags.Var(&symbol, "symbol", "instrument symbol")
	flags.Var(&eventID, "event-id", "economic event UUID")
	if err := flags.Parse(arguments); err != nil {
		return unlinkWatchlistEventCommand{}, invalidWatchlistArguments(
			"unlink-watchlist-event", unlinkWatchlistEventUsage, err,
		)
	}
	if flags.NArg() != 0 {
		return unlinkWatchlistEventCommand{}, invalidWatchlistArguments(
			"unlink-watchlist-event", unlinkWatchlistEventUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}
	for _, required := range []struct {
		name  string
		value *singleString
	}{{"id", &id}, {"symbol", &symbol}, {"event-id", &eventID}} {
		if !required.value.provided {
			return unlinkWatchlistEventCommand{}, invalidWatchlistArguments(
				"unlink-watchlist-event", unlinkWatchlistEventUsage,
				fmt.Errorf("--%s is required", required.name),
			)
		}
	}
	if !validWatchlistID(id.value) {
		return unlinkWatchlistEventCommand{}, invalidWatchlistArguments(
			"unlink-watchlist-event", unlinkWatchlistEventUsage, fmt.Errorf("--id must be a UUID"),
		)
	}
	if !validWatchlistID(eventID.value) {
		return unlinkWatchlistEventCommand{}, invalidWatchlistArguments(
			"unlink-watchlist-event", unlinkWatchlistEventUsage, fmt.Errorf("--event-id must be a UUID"),
		)
	}
	symbol.value = watchlist.NormalizeInstrumentSymbol(symbol.value)
	if symbol.value == "" {
		return unlinkWatchlistEventCommand{}, invalidWatchlistArguments(
			"unlink-watchlist-event", unlinkWatchlistEventUsage, fmt.Errorf("--symbol must not be blank"),
		)
	}
	return unlinkWatchlistEventCommand{
		watchlistID: id.value,
		symbol:      symbol.value,
		eventID:     eventID.value,
	}, nil
}

func runUnlinkWatchlistEvent(
	ctx context.Context,
	repository watchlistEventLinkDeleter,
	command unlinkWatchlistEventCommand,
) error {
	if err := repository.DeleteEventLink(
		ctx, command.watchlistID, command.symbol, command.eventID,
	); err != nil {
		return fmt.Errorf("unlink watchlist event: %w", err)
	}
	return nil
}
