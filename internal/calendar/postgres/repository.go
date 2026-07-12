// Package postgres persists normalized economic calendar records in PostgreSQL.
package postgres

import (
	"context"
	"errors"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/jackc/pgx/v5"
)

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Repository persists normalized economic events.
type Repository struct {
	db DB
}

var _ calendar.Repository = (*Repository)(nil)

// NewRepository returns an economic-event repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}

	return &Repository{db: db}, nil
}

func scanEvent(row pgx.Row) (calendar.StoredEvent, error) {
	var event calendar.StoredEvent
	err := row.Scan(
		&event.ID,
		&event.Source,
		&event.ExternalEventID,
		&event.Name,
		&event.Region,
		&event.Type,
		&event.ScheduledAt,
		&event.SourceURL,
		&event.RetrievedAt,
		&event.CreatedAt,
		&event.UpdatedAt,
		&event.CreatedBy,
		&event.UpdatedBy,
	)
	if err != nil {
		return calendar.StoredEvent{}, err
	}
	event.ScheduledAt = event.ScheduledAt.UTC()
	event.RetrievedAt = event.RetrievedAt.UTC()
	event.CreatedAt = event.CreatedAt.UTC()
	event.UpdatedAt = event.UpdatedAt.UTC()
	return event, nil
}

const eventColumns = `
    id::text,
    source,
    external_event_id,
    name,
    region,
    event_type,
    scheduled_at,
    source_url,
    retrieved_at,
    created_at,
    updated_at,
    created_by,
    updated_by`
