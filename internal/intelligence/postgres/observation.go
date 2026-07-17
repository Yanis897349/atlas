package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

// StoreObservation appends a newer changed source observation revision.
// Unchanged, older, and equal-time snapshots return the latest stored revision.
func (repository *Repository) StoreObservation(
	ctx context.Context,
	observation intelligence.Observation,
	actor string,
) (intelligence.StoredObservation, error) {
	observation, actor, err := normalizeAndValidateObservation(observation, actor)
	if err != nil {
		return intelligence.StoredObservation{}, err
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return intelligence.StoredObservation{}, fmt.Errorf("begin economic event observation storage: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	var lockedEventID string
	if err := transaction.QueryRow(
		ctx,
		lockEconomicEventForObservationStorageSQL,
		observation.EconomicEventID,
	).Scan(&lockedEventID); err != nil {
		return intelligence.StoredObservation{}, fmt.Errorf("lock economic event for observation storage: %w", err)
	}

	latestObservedAt, watermarkExists, err := loadObservationWatermark(
		ctx,
		transaction,
		lockedEventID,
		observation.Source,
		observation.SourceObservationID,
	)
	if err != nil {
		return intelligence.StoredObservation{}, err
	}

	var stored intelligence.StoredObservation
	if !watermarkExists {
		stored, err = insertObservationRevision(
			ctx,
			transaction,
			lockedEventID,
			observation,
			actor,
		)
		if err != nil {
			return intelligence.StoredObservation{}, err
		}
		if err := insertObservationWatermark(
			ctx,
			transaction,
			lockedEventID,
			observation,
			actor,
		); err != nil {
			return intelligence.StoredObservation{}, err
		}
	} else {
		stored, err = loadLatestObservation(
			ctx,
			transaction,
			lockedEventID,
			observation.Source,
			observation.SourceObservationID,
		)
		if err != nil {
			return intelligence.StoredObservation{}, err
		}

		if observation.ObservedAt.After(latestObservedAt) {
			if !sameObservationPayload(observation, stored.Observation) {
				stored, err = insertObservationRevision(
					ctx,
					transaction,
					lockedEventID,
					observation,
					actor,
				)
				if err != nil {
					return intelligence.StoredObservation{}, err
				}
			}
			if err := updateObservationWatermark(
				ctx,
				transaction,
				lockedEventID,
				observation,
				actor,
			); err != nil {
				return intelligence.StoredObservation{}, err
			}
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return intelligence.StoredObservation{}, fmt.Errorf("commit economic event observation storage: %w", err)
	}
	return stored, nil
}

func sameObservationPayload(left, right intelligence.Observation) bool {
	return left.SourceURL == right.SourceURL &&
		optionalStringEqual(left.Consensus, right.Consensus) &&
		optionalStringEqual(left.Previous, right.Previous) &&
		optionalStringEqual(left.Actual, right.Actual)
}

func optionalStringEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

const lockEconomicEventForObservationStorageSQL = `
SELECT id::text
FROM economic_events
WHERE id = $1
FOR NO KEY UPDATE`
