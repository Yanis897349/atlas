package app

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

const deleteWatchlistUsage = "usage: atlas delete-watchlist --id <uuid>"

type deleteWatchlistCommand struct {
	id string
}

func parseDeleteWatchlistCommand(arguments []string) (deleteWatchlistCommand, error) {
	id, err := parseRequiredWatchlistID("delete-watchlist", deleteWatchlistUsage, arguments)
	if err != nil {
		return deleteWatchlistCommand{}, err
	}
	return deleteWatchlistCommand{id: id}, nil
}

func runDeleteWatchlist(
	ctx context.Context,
	repository watchlist.Persistence,
	command deleteWatchlistCommand,
) error {
	if err := repository.DeleteWatchlist(ctx, command.id); err != nil {
		return fmt.Errorf("delete watchlist: %w", err)
	}
	return nil
}
