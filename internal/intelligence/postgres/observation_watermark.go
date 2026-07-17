package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/jackc/pgx/v5"
)

func loadObservationWatermark(
	ctx context.Context,
	transaction pgx.Tx,
	eventID string,
	source string,
	sourceObservationID string,
) (time.Time, bool, error) {
	var latestObservedAt time.Time
	err := transaction.QueryRow(
		ctx,
		observationWatermarkSQL,
		eventID,
		source,
		sourceObservationID,
	).Scan(&latestObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("load economic event observation watermark: %w", err)
	}
	return latestObservedAt.UTC(), true, nil
}

func insertObservationWatermark(
	ctx context.Context,
	transaction pgx.Tx,
	eventID string,
	observation intelligence.Observation,
	actor string,
) error {
	result, err := transaction.Exec(
		ctx,
		insertObservationWatermarkSQL,
		eventID,
		observation.Source,
		observation.SourceObservationID,
		observation.ObservedAt,
		actor,
	)
	if err != nil {
		return fmt.Errorf("insert economic event observation watermark: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("insert economic event observation watermark: affected %d rows", result.RowsAffected())
	}
	return nil
}

func updateObservationWatermark(
	ctx context.Context,
	transaction pgx.Tx,
	eventID string,
	observation intelligence.Observation,
	actor string,
) error {
	result, err := transaction.Exec(
		ctx,
		updateObservationWatermarkSQL,
		eventID,
		observation.Source,
		observation.SourceObservationID,
		observation.ObservedAt,
		actor,
	)
	if err != nil {
		return fmt.Errorf("update economic event observation watermark: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("update economic event observation watermark: affected %d rows", result.RowsAffected())
	}
	return nil
}

const observationWatermarkSQL = `
SELECT latest_observed_at
FROM economic_event_observation_watermarks
WHERE economic_event_id = $1
  AND source = $2
  AND source_observation_id = $3
FOR UPDATE`

const insertObservationWatermarkSQL = `
INSERT INTO economic_event_observation_watermarks (
    economic_event_id,
    source,
    source_observation_id,
    latest_observed_at,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $5, $5)`

const updateObservationWatermarkSQL = `
UPDATE economic_event_observation_watermarks
SET latest_observed_at = $4,
    updated_at = statement_timestamp(),
    updated_by = $5
WHERE economic_event_id = $1
  AND source = $2
  AND source_observation_id = $3
  AND latest_observed_at < $4`
