package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

type watchlistOutput struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Symbols   []string `json:"symbols"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	CreatedBy string   `json:"created_by"`
	UpdatedBy string   `json:"updated_by"`
}

func runCreateWatchlist(
	ctx context.Context,
	repository watchlistCreator,
	stdout io.Writer,
	command createWatchlistCommand,
) error {
	stored, err := repository.CreateWatchlist(ctx, command.definition, command.actor)
	if err != nil {
		return fmt.Errorf("create watchlist: %w", err)
	}
	return encodeCommandJSON(stdout, "created watchlist", newWatchlistOutput(stored))
}

func runUpdateWatchlist(
	ctx context.Context,
	repository watchlistUpdater,
	stdout io.Writer,
	command updateWatchlistCommand,
) error {
	stored, err := repository.UpdateWatchlist(ctx, command.id, command.definition, command.actor)
	if err != nil {
		return fmt.Errorf("update watchlist: %w", err)
	}
	return encodeCommandJSON(stdout, "updated watchlist", newWatchlistOutput(stored))
}

func runWatchlists(
	ctx context.Context,
	repository watchlistLister,
	stdout io.Writer,
	query watchlistsQuery,
) error {
	stored, err := repository.Watchlists(ctx, query.limit)
	if err != nil {
		return fmt.Errorf("retrieve watchlists: %w", err)
	}

	output := make([]watchlistOutput, 0, len(stored))
	for _, item := range stored {
		output = append(output, newWatchlistOutput(item))
	}
	return encodeCommandJSON(stdout, "watchlists", output)
}

func runWatchlist(
	ctx context.Context,
	repository watchlistLookup,
	stdout io.Writer,
	query watchlistQuery,
) error {
	stored, err := repository.Watchlist(ctx, query.id)
	if err != nil {
		return fmt.Errorf("retrieve watchlist: %w", err)
	}
	return encodeCommandJSON(stdout, "watchlist", newWatchlistOutput(stored))
}

func newWatchlistOutput(stored watchlist.StoredWatchlist) watchlistOutput {
	return watchlistOutput{
		ID:        stored.ID,
		Name:      stored.Name,
		Symbols:   append([]string(nil), stored.Symbols...),
		CreatedAt: formatOutputTime(stored.CreatedAt),
		UpdatedAt: formatOutputTime(stored.UpdatedAt),
		CreatedBy: stored.CreatedBy,
		UpdatedBy: stored.UpdatedBy,
	}
}
