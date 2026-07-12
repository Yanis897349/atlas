package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
)

// CreateEventLinks atomically creates or loads classified watchlist event associations.
func (repository *Repository) CreateEventLinks(
	ctx context.Context,
	watchlistID string,
	classifications []watchlist.EventRelevance,
	actor string,
) ([]watchlist.StoredEventLink, error) {
	classifications, actor, err := normalizeAndValidateEventLinks(watchlistID, classifications, actor)
	if err != nil {
		return nil, err
	}

	links := make([]watchlist.StoredEventLink, 0, len(classifications))
	if len(classifications) == 0 {
		return links, nil
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin watchlist event link creation: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	for index, classification := range classifications {
		link, createErr := createOrLoadEventLink(
			ctx,
			transaction,
			watchlistID,
			classification.Symbol,
			classification.Event.ID,
			actor,
		)
		if createErr != nil {
			return nil, fmt.Errorf("create watchlist event link %d: %w", index, createErr)
		}
		links = append(links, link)
	}

	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit watchlist event link creation: %w", err)
	}
	return links, nil
}

func normalizeAndValidateEventLinks(
	watchlistID string,
	classifications []watchlist.EventRelevance,
	actor string,
) ([]watchlist.EventRelevance, string, error) {
	if err := validateWatchlistID(watchlistID); err != nil {
		return nil, "", err
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, "", errors.New("actor is required")
	}

	normalized := make([]watchlist.EventRelevance, 0, len(classifications))
	seen := make(map[string]struct{}, len(classifications))
	for index, classification := range classifications {
		if !classification.Relevant {
			return nil, "", fmt.Errorf("classification %d must be relevant", index)
		}
		classification.Symbol = watchlist.NormalizeInstrumentSymbol(classification.Symbol)
		if classification.Symbol == "" {
			return nil, "", fmt.Errorf("classification %d instrument symbol is required", index)
		}
		if err := validateEventID(classification.Event.ID); err != nil {
			return nil, "", fmt.Errorf("classification %d: %w", index, err)
		}

		key := classification.Symbol + "\x00" + classification.Event.ID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, classification)
	}
	return normalized, actor, nil
}

func createOrLoadEventLink(
	ctx context.Context,
	transaction pgx.Tx,
	watchlistID string,
	symbol string,
	eventID string,
	actor string,
) (watchlist.StoredEventLink, error) {
	link, err := scanEventLink(transaction.QueryRow(
		ctx,
		createEventLinkIdempotentSQL,
		watchlistID,
		symbol,
		eventID,
		actor,
	))
	if err == nil {
		return link, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return watchlist.StoredEventLink{}, err
	}

	link, err = scanEventLink(transaction.QueryRow(
		ctx,
		eventLinkByReferenceSQL,
		watchlistID,
		symbol,
		eventID,
	))
	if err != nil {
		return watchlist.StoredEventLink{}, err
	}
	return link, nil
}

const createEventLinkIdempotentSQL = `
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
    ON CONFLICT (watchlist_instrument_id, economic_event_id) DO NOTHING
    RETURNING *
)
SELECT ` + eventLinkColumns + `
FROM inserted_link AS link
JOIN watchlist_instruments AS instrument ON instrument.id = link.watchlist_instrument_id
JOIN economic_events AS event ON event.id = link.economic_event_id`

const eventLinkByReferenceSQL = `
SELECT ` + eventLinkColumns + `
FROM watchlist_event_links AS link
JOIN watchlist_instruments AS instrument ON instrument.id = link.watchlist_instrument_id
JOIN economic_events AS event ON event.id = link.economic_event_id
WHERE instrument.watchlist_id = $1
  AND instrument.symbol = $2
  AND event.id = $3`
