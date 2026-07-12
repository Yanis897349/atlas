package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

// EventLinks returns a bounded chronological list for one watchlist instrument.
func (repository *Repository) EventLinks(
	ctx context.Context,
	watchlistID string,
	symbol string,
	limit int,
) ([]watchlist.StoredEventLink, error) {
	symbol, err := normalizeAndValidateEventLinksQuery(watchlistID, symbol, limit)
	if err != nil {
		return nil, err
	}
	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin watchlist event link retrieval: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	var instrumentID string
	if err := transaction.QueryRow(ctx, resolveWatchlistInstrumentSQL, watchlistID, symbol).Scan(&instrumentID); err != nil {
		return nil, fmt.Errorf("resolve watchlist instrument: %w", err)
	}
	rows, err := transaction.Query(ctx, eventLinksSQL, instrumentID, limit)
	if err != nil {
		return nil, fmt.Errorf("query watchlist event links: %w", err)
	}
	defer rows.Close()

	links := make([]watchlist.StoredEventLink, 0, limit)
	for rows.Next() {
		link, scanErr := scanEventLink(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan watchlist event link: %w", scanErr)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watchlist event links: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit watchlist event link retrieval: %w", err)
	}
	return links, nil
}

const resolveWatchlistInstrumentSQL = `
SELECT id::text
FROM watchlist_instruments
WHERE watchlist_id = $1 AND symbol = $2
FOR KEY SHARE`

const eventLinksSQL = `
SELECT ` + eventLinkColumns + `
FROM watchlist_event_links AS link
JOIN watchlist_instruments AS instrument ON instrument.id = link.watchlist_instrument_id
JOIN economic_events AS event ON event.id = link.economic_event_id
WHERE link.watchlist_instrument_id = $1
ORDER BY event.scheduled_at ASC, event.id ASC
LIMIT $2`
