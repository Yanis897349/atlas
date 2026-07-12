package watchlistcmd

import (
	"flag"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

const (
	watchlistUsage  = "usage: atlas watchlist --id <uuid>"
	watchlistsUsage = "usage: atlas watchlists --limit <1-100>"
)

type watchlistsQuery struct {
	limit int
}

type watchlistQuery struct {
	id string
}

func parseWatchlistsQuery(arguments []string) (watchlistsQuery, error) {
	flags := flag.NewFlagSet("watchlists", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var limit int
	flags.IntVar(&limit, "limit", 0, "maximum watchlist count")
	if err := flags.Parse(arguments); err != nil {
		return watchlistsQuery{}, invalidWatchlistArguments("watchlists", watchlistsUsage, err)
	}
	if flags.NArg() != 0 {
		return watchlistsQuery{}, invalidWatchlistArguments(
			"watchlists", watchlistsUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}

	provided := false
	flags.Visit(func(flag *flag.Flag) {
		provided = flag.Name == "limit"
	})
	if !provided {
		return watchlistsQuery{}, invalidWatchlistArguments("watchlists", watchlistsUsage, fmt.Errorf("--limit is required"))
	}
	if limit < 1 || limit > watchlist.MaxWatchlistsLimit {
		return watchlistsQuery{}, invalidWatchlistArguments(
			"watchlists",
			watchlistsUsage,
			fmt.Errorf("--limit must be between 1 and %d", watchlist.MaxWatchlistsLimit),
		)
	}
	return watchlistsQuery{limit: limit}, nil
}

func parseWatchlistQuery(arguments []string) (watchlistQuery, error) {
	id, err := parseRequiredWatchlistID("watchlist", watchlistUsage, arguments)
	if err != nil {
		return watchlistQuery{}, err
	}
	return watchlistQuery{id: id}, nil
}

func parseRequiredWatchlistID(commandName, usage string, arguments []string) (string, error) {
	flags := flag.NewFlagSet(commandName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var id singleString
	flags.Var(&id, "id", "watchlist UUID")
	if err := flags.Parse(arguments); err != nil {
		return "", invalidWatchlistArguments(commandName, usage, err)
	}
	if flags.NArg() != 0 {
		return "", invalidWatchlistArguments(
			commandName, usage, fmt.Errorf("unexpected positional arguments"),
		)
	}
	if !id.provided {
		return "", invalidWatchlistArguments(commandName, usage, fmt.Errorf("--id is required"))
	}

	if !validWatchlistID(id.value) {
		return "", invalidWatchlistArguments(
			commandName, usage, fmt.Errorf("--id must be a UUID"),
		)
	}
	return id.value, nil
}
