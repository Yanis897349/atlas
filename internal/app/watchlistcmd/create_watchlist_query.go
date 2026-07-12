package watchlistcmd

import (
	"flag"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

const createWatchlistUsage = "usage: atlas create-watchlist --name <name> --actor <actor> --symbol <symbol> [--symbol <symbol> ...]"

type createWatchlistCommand struct {
	definition watchlist.Definition
	actor      string
}

func parseCreateWatchlistCommand(arguments []string) (createWatchlistCommand, error) {
	flags := flag.NewFlagSet("create-watchlist", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var name, actor string
	var symbols repeatedStrings
	flags.StringVar(&name, "name", "", "watchlist name")
	flags.StringVar(&actor, "actor", "", "audit actor")
	flags.Var(&symbols, "symbol", "instrument symbol")
	if err := flags.Parse(arguments); err != nil {
		return createWatchlistCommand{}, invalidWatchlistArguments("create-watchlist", createWatchlistUsage, err)
	}
	if flags.NArg() != 0 {
		return createWatchlistCommand{}, invalidWatchlistArguments(
			"create-watchlist", createWatchlistUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}

	provided := make(map[string]bool, 3)
	flags.Visit(func(flag *flag.Flag) {
		provided[flag.Name] = true
	})
	for _, required := range []string{"name", "actor", "symbol"} {
		if !provided[required] {
			return createWatchlistCommand{}, invalidWatchlistArguments(
				"create-watchlist", createWatchlistUsage, fmt.Errorf("--%s is required", required),
			)
		}
	}

	definition, actor, err := normalizeWatchlistCommandDefinition(
		"create-watchlist", createWatchlistUsage, name, actor, symbols,
	)
	if err != nil {
		return createWatchlistCommand{}, err
	}
	return createWatchlistCommand{definition: definition, actor: actor}, nil
}
