package app

import (
	"fmt"
	"strings"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

func normalizeWatchlistCommandDefinition(
	commandName, usage, name, actor string,
	symbols []string,
) (watchlist.Definition, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return watchlist.Definition{}, "", invalidWatchlistArguments(
			commandName, usage, fmt.Errorf("--name must not be blank"),
		)
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return watchlist.Definition{}, "", invalidWatchlistArguments(
			commandName, usage, fmt.Errorf("--actor must not be blank"),
		)
	}

	normalizedSymbols := make([]string, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for index, symbol := range symbols {
		symbol = watchlist.NormalizeInstrumentSymbol(symbol)
		if symbol == "" {
			return watchlist.Definition{}, "", invalidWatchlistArguments(
				commandName, usage, fmt.Errorf("--symbol %d must not be blank", index+1),
			)
		}
		if _, exists := seen[symbol]; exists {
			return watchlist.Definition{}, "", invalidWatchlistArguments(
				commandName, usage, fmt.Errorf("--symbol %q is duplicated", symbol),
			)
		}
		seen[symbol] = struct{}{}
		normalizedSymbols[index] = symbol
	}
	return watchlist.Definition{Name: name, Symbols: normalizedSymbols}, actor, nil
}
