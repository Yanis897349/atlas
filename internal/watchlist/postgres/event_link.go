package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
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

	link, err := scanEventLink(repository.db.QueryRow(
		ctx,
		createEventLinkSQL,
		watchlistID,
		symbol,
		eventID,
		actor,
	))
	if err != nil {
		return watchlist.StoredEventLink{}, fmt.Errorf("create watchlist event link: %w", err)
	}
	return link, nil
}

// DeleteEventLink deletes one association between a watchlist instrument and an economic event.
func (repository *Repository) DeleteEventLink(
	ctx context.Context,
	watchlistID string,
	symbol string,
	eventID string,
) error {
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

func scanEventLink(row pgx.Row) (watchlist.StoredEventLink, error) {
	var link watchlist.StoredEventLink
	err := row.Scan(
		&link.ID,
		&link.WatchlistID,
		&link.Symbol,
		&link.CreatedAt,
		&link.UpdatedAt,
		&link.CreatedBy,
		&link.UpdatedBy,
		&link.Event.ID,
		&link.Event.Source,
		&link.Event.ExternalEventID,
		&link.Event.Name,
		&link.Event.Region,
		&link.Event.Type,
		&link.Event.ScheduledAt,
		&link.Event.SourceURL,
		&link.Event.RetrievedAt,
		&link.Event.CreatedAt,
		&link.Event.UpdatedAt,
		&link.Event.CreatedBy,
		&link.Event.UpdatedBy,
	)
	if err != nil {
		return watchlist.StoredEventLink{}, err
	}

	link.CreatedAt = link.CreatedAt.UTC()
	link.UpdatedAt = link.UpdatedAt.UTC()
	link.Event.ScheduledAt = link.Event.ScheduledAt.UTC()
	link.Event.RetrievedAt = link.Event.RetrievedAt.UTC()
	link.Event.CreatedAt = link.Event.CreatedAt.UTC()
	link.Event.UpdatedAt = link.Event.UpdatedAt.UTC()
	return link, nil
}

const eventLinkColumns = `
    link.id::text,
    instrument.watchlist_id::text,
    instrument.symbol,
    link.created_at,
    link.updated_at,
    link.created_by,
    link.updated_by,
    event.id::text,
    event.source,
    event.external_event_id,
    event.name,
    event.region,
    event.event_type,
    event.scheduled_at,
    event.source_url,
    event.retrieved_at,
    event.created_at,
    event.updated_at,
    event.created_by,
    event.updated_by`

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
