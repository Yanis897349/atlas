package watchlistcmd

import (
	"flag"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

const updateWatchlistUsage = "usage: atlas update-watchlist --id <uuid> --name <name> --actor <actor> --symbol <symbol> [--symbol <symbol> ...]"

type updateWatchlistCommand struct {
	id         string
	definition watchlist.Definition
	actor      string
}

func parseUpdateWatchlistCommand(arguments []string) (updateWatchlistCommand, error) {
	flags := flag.NewFlagSet("update-watchlist", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var id singleString
	var name, actor string
	var symbols repeatedStrings
	flags.Var(&id, "id", "watchlist UUID")
	flags.StringVar(&name, "name", "", "watchlist name")
	flags.StringVar(&actor, "actor", "", "audit actor")
	flags.Var(&symbols, "symbol", "instrument symbol")
	if err := flags.Parse(arguments); err != nil {
		return updateWatchlistCommand{}, invalidWatchlistArguments("update-watchlist", updateWatchlistUsage, err)
	}
	if flags.NArg() != 0 {
		return updateWatchlistCommand{}, invalidWatchlistArguments(
			"update-watchlist", updateWatchlistUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}

	provided := make(map[string]bool, 4)
	flags.Visit(func(flag *flag.Flag) {
		provided[flag.Name] = true
	})
	for _, required := range []string{"id", "name", "actor", "symbol"} {
		if !provided[required] {
			return updateWatchlistCommand{}, invalidWatchlistArguments(
				"update-watchlist", updateWatchlistUsage, fmt.Errorf("--%s is required", required),
			)
		}
	}
	if !validWatchlistID(id.value) {
		return updateWatchlistCommand{}, invalidWatchlistArguments(
			"update-watchlist", updateWatchlistUsage, fmt.Errorf("--id must be a UUID"),
		)
	}
	definition, actor, err := normalizeWatchlistCommandDefinition(
		"update-watchlist", updateWatchlistUsage, name, actor, symbols,
	)
	if err != nil {
		return updateWatchlistCommand{}, err
	}
	return updateWatchlistCommand{id: id.value, definition: definition, actor: actor}, nil
}
