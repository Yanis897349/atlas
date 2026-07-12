package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

// WatchlistEventCandidates returns supported events within the inclusive time window.
func (repository *Repository) WatchlistEventCandidates(
	ctx context.Context,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]calendar.StoredEvent, error) {
	if err := validateWatchlistEventCandidatesQuery(windowStart, windowEnd, limit); err != nil {
		return nil, err
	}

	rows, err := repository.db.Query(
		ctx,
		watchlistEventCandidatesSQL,
		windowStart.UTC(),
		windowEnd.UTC(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query watchlist event candidates: %w", err)
	}
	defer rows.Close()

	events := make([]calendar.StoredEvent, 0, limit)
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan watchlist event candidate: %w", scanErr)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watchlist event candidates: %w", err)
	}

	return events, nil
}

func validateWatchlistEventCandidatesQuery(windowStart time.Time, windowEnd time.Time, limit int) error {
	if windowStart.IsZero() {
		return errors.New("window start is required")
	}
	if windowEnd.IsZero() {
		return errors.New("window end is required")
	}
	if windowEnd.Before(windowStart) {
		return errors.New("window end must not be before window start")
	}
	if limit < 1 || limit > calendar.MaxWatchlistEventCandidatesLimit {
		return fmt.Errorf("limit must be between 1 and %d", calendar.MaxWatchlistEventCandidatesLimit)
	}

	return nil
}

const watchlistEventCandidatesSQL = `
SELECT ` + eventColumns + `
FROM economic_events
WHERE region IN ('united_states', 'eurozone')
  AND scheduled_at >= $1
  AND scheduled_at <= $2
ORDER BY scheduled_at, id
LIMIT $3`
