package postgres

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5/pgtype"
)

func normalizeAndValidateDefinition(
	definition watchlist.Definition,
	actor string,
) (watchlist.Definition, string, error) {
	definition.Name = strings.TrimSpace(definition.Name)
	actor = strings.TrimSpace(actor)
	if definition.Name == "" {
		return watchlist.Definition{}, "", errors.New("name is required")
	}
	if actor == "" {
		return watchlist.Definition{}, "", errors.New("actor is required")
	}
	if len(definition.Symbols) == 0 {
		return watchlist.Definition{}, "", errors.New("at least one instrument symbol is required")
	}

	symbols := make([]string, len(definition.Symbols))
	seen := make(map[string]struct{}, len(definition.Symbols))
	for index, symbol := range definition.Symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			return watchlist.Definition{}, "", fmt.Errorf("instrument symbol %d is required", index)
		}
		if _, exists := seen[symbol]; exists {
			return watchlist.Definition{}, "", fmt.Errorf("instrument symbol %q is duplicated", symbol)
		}
		seen[symbol] = struct{}{}
		symbols[index] = symbol
	}
	definition.Symbols = symbols
	return definition, actor, nil
}

func validateWatchlistID(id string) error {
	var value pgtype.UUID
	if value.Scan(id) != nil || !value.Valid {
		return errors.New("watchlist ID must be a UUID")
	}
	return nil
}

func validateWatchlistsLimit(limit int) error {
	if limit < 1 || limit > watchlist.MaxWatchlistsLimit {
		return fmt.Errorf("limit must be between 1 and %d", watchlist.MaxWatchlistsLimit)
	}
	return nil
}
