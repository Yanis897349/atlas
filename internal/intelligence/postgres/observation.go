package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/jackc/pgx/v5"
)

// StoreObservation inserts a source observation or replaces it with a newer snapshot.
// Source identity and creation audit fields remain immutable after the first insert.
func (repository *Repository) StoreObservation(
	ctx context.Context,
	observation intelligence.Observation,
	actor string,
) (intelligence.StoredObservation, error) {
	observation, actor, err := normalizeAndValidateObservation(observation, actor)
	if err != nil {
		return intelligence.StoredObservation{}, err
	}

	stored, err := scanObservation(repository.db.QueryRow(
		ctx,
		upsertObservationSQL,
		observation.EconomicEventID,
		observation.Source,
		observation.SourceObservationID,
		observation.SourceURL,
		observation.ObservedAt,
		observation.Consensus,
		observation.Previous,
		observation.Actual,
		actor,
	))
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return intelligence.StoredObservation{}, fmt.Errorf("upsert economic event observation: %w", err)
	}

	stored, err = scanObservation(repository.db.QueryRow(
		ctx,
		selectObservationSQL,
		observation.EconomicEventID,
		observation.Source,
		observation.SourceObservationID,
	))
	if err != nil {
		return intelligence.StoredObservation{}, fmt.Errorf("load unchanged economic event observation: %w", err)
	}
	return stored, nil
}

const upsertObservationSQL = `
INSERT INTO economic_event_observations (
    economic_event_id,
    source,
    source_observation_id,
    source_url,
    observed_at,
    consensus_value,
    previous_value,
    actual_value,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
ON CONFLICT (economic_event_id, source, source_observation_id) DO UPDATE
SET source_url = EXCLUDED.source_url,
    observed_at = EXCLUDED.observed_at,
    consensus_value = EXCLUDED.consensus_value,
    previous_value = EXCLUDED.previous_value,
    actual_value = EXCLUDED.actual_value,
    updated_at = statement_timestamp(),
    updated_by = EXCLUDED.updated_by
WHERE EXCLUDED.observed_at > economic_event_observations.observed_at
RETURNING ` + observationColumns

const selectObservationSQL = `
SELECT ` + observationColumns + `
FROM economic_event_observations
WHERE economic_event_id = $1
  AND source = $2
  AND source_observation_id = $3`
