package postgres

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

// ObservationRevisions returns bounded immutable revisions for one exact source observation identity.
func (repository *Repository) ObservationRevisions(
	ctx context.Context,
	eventID string,
	source string,
	sourceObservationID string,
	limit int,
) ([]intelligence.StoredObservation, error) {
	eventID, source, sourceObservationID, err := normalizeAndValidateObservationRevisionsQuery(
		eventID,
		source,
		sourceObservationID,
		limit,
	)
	if err != nil {
		return nil, err
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin economic event observation revision retrieval: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	var lockedEventID string
	if err := transaction.QueryRow(ctx, lockEconomicEventSQL, eventID).Scan(&lockedEventID); err != nil {
		return nil, fmt.Errorf("lock economic event for observation revision retrieval: %w", err)
	}

	rows, err := transaction.Query(
		ctx,
		observationRevisionsSQL,
		lockedEventID,
		source,
		sourceObservationID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query economic event observation revisions: %w", err)
	}
	defer rows.Close()

	revisions := make([]intelligence.StoredObservation, 0, limit)
	for rows.Next() {
		revision, scanErr := scanObservation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan economic event observation revision: %w", scanErr)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate economic event observation revisions: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit economic event observation revision retrieval: %w", err)
	}
	return revisions, nil
}

const observationRevisionsSQL = `
SELECT ` + observationColumns + `
FROM economic_event_observations
WHERE economic_event_id = $1
  AND source = $2
  AND source_observation_id = $3
ORDER BY observed_at DESC, id ASC
LIMIT $4`
