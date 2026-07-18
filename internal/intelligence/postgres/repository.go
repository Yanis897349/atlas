// Package postgres persists economic-event intelligence in PostgreSQL.
package postgres

import (
	"context"
	"errors"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Repository persists and retrieves economic-event intelligence.
type Repository struct {
	db DB
}

var (
	_ intelligence.ObservationWriter         = (*Repository)(nil)
	_ intelligence.ObservationReader         = (*Repository)(nil)
	_ intelligence.ObservationRevisionReader = (*Repository)(nil)
)

// NewRepository returns an economic-event intelligence repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}
	return &Repository{db: db}, nil
}

func scanObservation(row pgx.Row) (intelligence.StoredObservation, error) {
	var (
		observation                 intelligence.StoredObservation
		consensus, previous, actual pgtype.Text
	)
	if err := row.Scan(
		&observation.ID,
		&observation.EconomicEventID,
		&observation.Source,
		&observation.SourceObservationID,
		&observation.SourceURL,
		&observation.ObservedAt,
		&consensus,
		&previous,
		&actual,
		&observation.CreatedAt,
		&observation.UpdatedAt,
		&observation.CreatedBy,
		&observation.UpdatedBy,
	); err != nil {
		return intelligence.StoredObservation{}, err
	}

	observation.Consensus = optionalText(consensus)
	observation.Previous = optionalText(previous)
	observation.Actual = optionalText(actual)
	observation.ObservedAt = observation.ObservedAt.UTC()
	observation.CreatedAt = observation.CreatedAt.UTC()
	observation.UpdatedAt = observation.UpdatedAt.UTC()
	return observation, nil
}

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

const observationColumns = `
    id::text,
    economic_event_id::text,
    source,
    source_observation_id,
    source_url,
    observed_at,
    consensus_value,
    previous_value,
    actual_value,
    created_at,
    updated_at,
    created_by,
    updated_by`
