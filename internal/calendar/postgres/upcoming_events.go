package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

// UpcomingEvents returns events for region within the inclusive time window.
func (repository *Repository) UpcomingEvents(
	ctx context.Context,
	region calendar.Region,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]calendar.StoredEvent, error) {
	if err := validateUpcomingEventsQuery(region, windowStart, windowEnd, limit); err != nil {
		return nil, err
	}
	rows, err := repository.db.Query(ctx, upcomingEventsSQL, region, windowStart.UTC(), windowEnd.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("query upcoming economic events: %w", err)
	}
	defer rows.Close()

	events := make([]calendar.StoredEvent, 0, limit)
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan upcoming economic event: %w", scanErr)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upcoming economic events: %w", err)
	}
	return events, nil
}

func validateUpcomingEventsQuery(region calendar.Region, windowStart, windowEnd time.Time, limit int) error {
	if !validRegion(region) {
		return fmt.Errorf("unsupported region %q", region)
	}
	if windowStart.IsZero() {
		return errors.New("window start is required")
	}
	if windowEnd.IsZero() {
		return errors.New("window end is required")
	}
	if windowEnd.Before(windowStart) {
		return errors.New("window end must not be before window start")
	}
	if limit < 1 || limit > calendar.MaxUpcomingEventsLimit {
		return fmt.Errorf("limit must be between 1 and %d", calendar.MaxUpcomingEventsLimit)
	}
	return nil
}

func validRegion(region calendar.Region) bool {
	return region == calendar.RegionUnitedStates || region == calendar.RegionEurozone
}

const upcomingEventsSQL = `
SELECT ` + eventColumns + `
FROM economic_events
WHERE region = $1
  AND scheduled_at >= $2
  AND scheduled_at <= $3
ORDER BY scheduled_at, id
LIMIT $4`
