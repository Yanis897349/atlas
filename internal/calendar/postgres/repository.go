// Package postgres persists normalized economic calendar records in PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/jackc/pgx/v5"
)

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Repository persists normalized economic events.
type Repository struct {
	db DB
}

// StoredEvent is a normalized economic event with its persistence metadata.
type StoredEvent struct {
	ID string
	calendar.Event
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

// NewRepository returns an economic-event repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}

	return &Repository{db: db}, nil
}

// UpsertEvent inserts an event or applies metadata from a newer retrieval.
// Source identity and creation audit fields remain immutable after the first insert.
func (repository *Repository) UpsertEvent(
	ctx context.Context,
	event calendar.Event,
	actor string,
) (StoredEvent, error) {
	actor = strings.TrimSpace(actor)
	event.Source = strings.TrimSpace(event.Source)
	event.ExternalEventID = strings.TrimSpace(event.ExternalEventID)
	if err := validateEvent(event, actor); err != nil {
		return StoredEvent{}, err
	}

	event.ScheduledAt = event.ScheduledAt.UTC()
	event.RetrievedAt = event.RetrievedAt.UTC()

	stored, err := scanEvent(repository.db.QueryRow(
		ctx,
		upsertEventSQL,
		event.Source,
		event.ExternalEventID,
		event.Name,
		event.Region,
		event.Type,
		event.ScheduledAt,
		event.SourceURL,
		event.RetrievedAt,
		actor,
	))
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return StoredEvent{}, fmt.Errorf("upsert economic event: %w", err)
	}

	stored, err = scanEvent(repository.db.QueryRow(
		ctx,
		selectEventSQL,
		event.Source,
		event.ExternalEventID,
	))
	if err != nil {
		return StoredEvent{}, fmt.Errorf("load unchanged economic event: %w", err)
	}

	return stored, nil
}

func scanEvent(row pgx.Row) (StoredEvent, error) {
	var event StoredEvent
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
	return event, err
}

func validateEvent(event calendar.Event, actor string) error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "source", value: event.Source},
		{name: "external event ID", value: event.ExternalEventID},
		{name: "name", value: event.Name},
		{name: "actor", value: actor},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}

	if !validRegion(event.Region) {
		return fmt.Errorf("unsupported region %q", event.Region)
	}
	if !validEventType(event.Type) {
		return fmt.Errorf("unsupported event type %q", event.Type)
	}
	if event.ScheduledAt.IsZero() {
		return errors.New("scheduled time is required")
	}
	parsedURL, err := url.Parse(event.SourceURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Hostname() == "" {
		return errors.New("source URL must be an absolute HTTP(S) URL")
	}
	if event.RetrievedAt.IsZero() {
		return errors.New("retrieved time is required")
	}

	return nil
}

func validRegion(region calendar.Region) bool {
	return region == calendar.RegionUnitedStates || region == calendar.RegionEurozone
}

func validEventType(eventType calendar.EventType) bool {
	switch eventType {
	case calendar.EventTypeInflation,
		calendar.EventTypeEmployment,
		calendar.EventTypeInterestRateDecision,
		calendar.EventTypeGDP,
		calendar.EventTypePMI,
		calendar.EventTypeRetailSales:
		return true
	default:
		return false
	}
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

const upsertEventSQL = `
INSERT INTO economic_events (
    source,
    external_event_id,
    name,
    region,
    event_type,
    scheduled_at,
    source_url,
    retrieved_at,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
ON CONFLICT (source, external_event_id) DO UPDATE
SET name = EXCLUDED.name,
    region = EXCLUDED.region,
    event_type = EXCLUDED.event_type,
    scheduled_at = EXCLUDED.scheduled_at,
    source_url = EXCLUDED.source_url,
    retrieved_at = EXCLUDED.retrieved_at,
    updated_at = statement_timestamp(),
    updated_by = EXCLUDED.updated_by
WHERE EXCLUDED.retrieved_at > economic_events.retrieved_at
RETURNING ` + eventColumns

const selectEventSQL = `
SELECT ` + eventColumns + `
FROM economic_events
WHERE source = $1 AND external_event_id = $2`
