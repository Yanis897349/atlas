package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	createWatchlistUsage = "usage: atlas create-watchlist --name <name> --actor <actor> --symbol <symbol> [--symbol <symbol> ...]"
	watchlistUsage       = "usage: atlas watchlist --id <uuid>"
	watchlistsUsage      = "usage: atlas watchlists --limit <1-100>"
)

type createWatchlistCommand struct {
	definition watchlist.Definition
	actor      string
}

type watchlistsQuery struct {
	limit int
}

type watchlistQuery struct {
	id string
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

type repeatedStrings []string

func (values *repeatedStrings) String() string {
	return strings.Join(*values, ",")
}

func (values *repeatedStrings) Set(value string) error {
	*values = append(*values, value)
	return nil
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

	name = strings.TrimSpace(name)
	if name == "" {
		return createWatchlistCommand{}, invalidWatchlistArguments(
			"create-watchlist", createWatchlistUsage, fmt.Errorf("--name must not be blank"),
		)
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return createWatchlistCommand{}, invalidWatchlistArguments(
			"create-watchlist", createWatchlistUsage, fmt.Errorf("--actor must not be blank"),
		)
	}

	normalizedSymbols := make([]string, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for index, symbol := range symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			return createWatchlistCommand{}, invalidWatchlistArguments(
				"create-watchlist", createWatchlistUsage, fmt.Errorf("--symbol %d must not be blank", index+1),
			)
		}
		if _, exists := seen[symbol]; exists {
			return createWatchlistCommand{}, invalidWatchlistArguments(
				"create-watchlist", createWatchlistUsage, fmt.Errorf("--symbol %q is duplicated", symbol),
			)
		}
		seen[symbol] = struct{}{}
		normalizedSymbols[index] = symbol
	}

	return createWatchlistCommand{
		definition: watchlist.Definition{Name: name, Symbols: normalizedSymbols},
		actor:      actor,
	}, nil
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
	flags := flag.NewFlagSet("watchlist", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var id singleString
	flags.Var(&id, "id", "watchlist UUID")
	if err := flags.Parse(arguments); err != nil {
		return watchlistQuery{}, invalidWatchlistArguments("watchlist", watchlistUsage, err)
	}
	if flags.NArg() != 0 {
		return watchlistQuery{}, invalidWatchlistArguments(
			"watchlist", watchlistUsage, fmt.Errorf("unexpected positional arguments"),
		)
	}
	if !id.provided {
		return watchlistQuery{}, invalidWatchlistArguments("watchlist", watchlistUsage, fmt.Errorf("--id is required"))
	}

	var parsed pgtype.UUID
	if err := parsed.Scan(id.value); err != nil || !parsed.Valid {
		return watchlistQuery{}, invalidWatchlistArguments(
			"watchlist", watchlistUsage, fmt.Errorf("--id must be a UUID"),
		)
	}
	return watchlistQuery{id: id.value}, nil
}

func invalidWatchlistArguments(commandName, usage string, err error) error {
	return fmt.Errorf("invalid %s arguments: %w; %s", commandName, err, usage)
}
