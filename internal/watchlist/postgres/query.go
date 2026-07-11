package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
)

// Watchlist returns one watchlist by UUID.
func (repository *Repository) Watchlist(ctx context.Context, id string) (watchlist.StoredWatchlist, error) {
	if err := validateWatchlistID(id); err != nil {
		return watchlist.StoredWatchlist{}, err
	}

	watchlists, err := repository.queryWatchlists(ctx, watchlistByIDSQL, id)
	if err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("query watchlist: %w", err)
	}
	if len(watchlists) == 0 {
		return watchlist.StoredWatchlist{}, fmt.Errorf("query watchlist: %w", pgx.ErrNoRows)
	}
	return watchlists[0], nil
}

// Watchlists returns a bounded newest-first list with stable UUID tie-breaking.
func (repository *Repository) Watchlists(ctx context.Context, limit int) ([]watchlist.StoredWatchlist, error) {
	if err := validateWatchlistsLimit(limit); err != nil {
		return nil, err
	}

	watchlists, err := repository.queryWatchlists(ctx, watchlistsSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("query watchlists: %w", err)
	}
	return watchlists, nil
}

func (repository *Repository) queryWatchlists(
	ctx context.Context,
	query string,
	arguments ...any,
) ([]watchlist.StoredWatchlist, error) {
	rows, err := repository.db.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	watchlists := make([]watchlist.StoredWatchlist, 0)
	currentID := ""
	for rows.Next() {
		var stored watchlist.StoredWatchlist
		var symbol string
		if err := rows.Scan(
			&stored.ID,
			&stored.Name,
			&stored.CreatedAt,
			&stored.UpdatedAt,
			&stored.CreatedBy,
			&stored.UpdatedBy,
			&symbol,
		); err != nil {
			return nil, fmt.Errorf("scan watchlist: %w", err)
		}
		stored.CreatedAt = stored.CreatedAt.UTC()
		stored.UpdatedAt = stored.UpdatedAt.UTC()

		if stored.ID != currentID {
			stored.Symbols = make([]string, 0, 1)
			watchlists = append(watchlists, stored)
			currentID = stored.ID
		}
		watchlists[len(watchlists)-1].Symbols = append(watchlists[len(watchlists)-1].Symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watchlists: %w", err)
	}
	return watchlists, nil
}

const watchlistColumns = `
    watchlist.id::text,
    watchlist.name,
    watchlist.created_at,
    watchlist.updated_at,
    watchlist.created_by,
    watchlist.updated_by,
    instrument.symbol`

const watchlistByIDSQL = `
SELECT ` + watchlistColumns + `
FROM watchlists AS watchlist
JOIN watchlist_instruments AS instrument ON instrument.watchlist_id = watchlist.id
WHERE watchlist.id = $1
ORDER BY instrument.position ASC`

const watchlistsSQL = `
WITH selected_watchlists AS (
    SELECT id, name, created_at, updated_at, created_by, updated_by
    FROM watchlists
    ORDER BY created_at DESC, id ASC
    LIMIT $1
)
SELECT ` + watchlistColumns + `
FROM selected_watchlists AS watchlist
JOIN watchlist_instruments AS instrument ON instrument.watchlist_id = watchlist.id
ORDER BY watchlist.created_at DESC, watchlist.id ASC, instrument.position ASC`
