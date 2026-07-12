package postgres

import (
	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
)

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
