package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

// EventObservations returns the bounded latest source observations for one canonical economic event.
func (repository *Repository) EventObservations(
	ctx context.Context,
	eventID string,
	limit int,
) ([]intelligence.StoredObservation, error) {
	eventID, err := normalizeAndValidateEventObservationsQuery(eventID, limit)
	if err != nil {
		return nil, err
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin economic event observation retrieval: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	var lockedEventID string
	if err := transaction.QueryRow(ctx, lockEconomicEventSQL, eventID).Scan(&lockedEventID); err != nil {
		return nil, fmt.Errorf("lock economic event for observation retrieval: %w", err)
	}

	rows, err := transaction.Query(ctx, eventObservationsSQL, lockedEventID, limit)
	if err != nil {
		return nil, fmt.Errorf("query economic event observations: %w", err)
	}
	defer rows.Close()

	observations := make([]intelligence.StoredObservation, 0, limit)
	for rows.Next() {
		observation, scanErr := scanObservation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan economic event observation: %w", scanErr)
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate economic event observations: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit economic event observation retrieval: %w", err)
	}
	return observations, nil
}

const lockEconomicEventSQL = `
SELECT id::text
FROM economic_events
WHERE id = $1
FOR KEY SHARE`

const eventObservationsSQL = `
SELECT ` + observationColumns + `
FROM (
    SELECT economic_event_observations.*,
           row_number() OVER (
               PARTITION BY source, source_observation_id
               ORDER BY observed_at DESC, id ASC
           ) AS revision_rank
    FROM economic_event_observations
    WHERE economic_event_id = $1
) AS latest_observations
WHERE revision_rank = 1
ORDER BY observed_at DESC, id ASC
LIMIT $2`
