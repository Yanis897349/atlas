package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/jackc/pgx/v5"
)

func loadLatestObservation(
	ctx context.Context,
	transaction pgx.Tx,
	eventID string,
	source string,
	sourceObservationID string,
) (intelligence.StoredObservation, error) {
	stored, err := scanObservation(transaction.QueryRow(
		ctx,
		latestObservationSQL,
		eventID,
		source,
		sourceObservationID,
	))
	if err != nil {
		return intelligence.StoredObservation{}, fmt.Errorf("load latest economic event observation: %w", err)
	}
	return stored, nil
}

func insertObservationRevision(
	ctx context.Context,
	transaction pgx.Tx,
	eventID string,
	observation intelligence.Observation,
	actor string,
) (intelligence.StoredObservation, error) {
	stored, err := scanObservation(transaction.QueryRow(
		ctx,
		insertObservationSQL,
		eventID,
		observation.Source,
		observation.SourceObservationID,
		observation.SourceURL,
		observation.ObservedAt,
		observation.Consensus,
		observation.Previous,
		observation.Actual,
		actor,
	))
	if err != nil {
		return intelligence.StoredObservation{}, fmt.Errorf("insert economic event observation revision: %w", err)
	}
	return stored, nil
}

const latestObservationSQL = `
SELECT ` + observationColumns + `
FROM economic_event_observations
WHERE economic_event_id = $1
  AND source = $2
  AND source_observation_id = $3
ORDER BY observed_at DESC, id ASC
LIMIT 1`

const insertObservationSQL = `
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
RETURNING ` + observationColumns
