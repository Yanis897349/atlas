package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

// CreateEventLink creates an immutable association between a watchlist instrument and an economic event.
func (repository *Repository) CreateEventLink(
	ctx context.Context,
	watchlistID string,
	symbol string,
	eventID string,
	actor string,
) (watchlist.StoredEventLink, error) {
	symbol, actor, err := normalizeAndValidateEventLink(watchlistID, symbol, eventID, actor)
	if err != nil {
		return watchlist.StoredEventLink{}, err
	}
	link, err := scanEventLink(repository.db.QueryRow(ctx, createEventLinkSQL, watchlistID, symbol, eventID, actor))
	if err != nil {
		return watchlist.StoredEventLink{}, fmt.Errorf("create watchlist event link: %w", err)
	}
	return link, nil
}

// DeleteEventLink deletes one association between a watchlist instrument and an economic event.
func (repository *Repository) DeleteEventLink(ctx context.Context, watchlistID, symbol, eventID string) error {
	symbol, err := normalizeAndValidateEventLinkReference(watchlistID, symbol, eventID)
	if err != nil {
		return err
	}
	var deletedID string
	if err := repository.db.QueryRow(ctx, deleteEventLinkSQL, watchlistID, symbol, eventID).Scan(&deletedID); err != nil {
		return fmt.Errorf("delete watchlist event link: %w", err)
	}
	return nil
}

const createEventLinkSQL = `
WITH inserted_link AS (
    INSERT INTO watchlist_event_links (
        watchlist_instrument_id,
        economic_event_id,
        created_by,
        updated_by
    )
    SELECT instrument.id, event.id, $4, $4
    FROM watchlist_instruments AS instrument
    CROSS JOIN economic_events AS event
    WHERE instrument.watchlist_id = $1
      AND instrument.symbol = $2
      AND event.id = $3
    RETURNING *
)
SELECT ` + eventLinkColumns + `
FROM inserted_link AS link
JOIN watchlist_instruments AS instrument ON instrument.id = link.watchlist_instrument_id
JOIN economic_events AS event ON event.id = link.economic_event_id`

const deleteEventLinkSQL = `
DELETE FROM watchlist_event_links AS link
USING watchlist_instruments AS instrument
WHERE link.watchlist_instrument_id = instrument.id
  AND instrument.watchlist_id = $1
  AND instrument.symbol = $2
  AND link.economic_event_id = $3
RETURNING link.id::text`
