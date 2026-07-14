package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/jackc/pgx/v5/pgtype"
)

// EconomicEvent returns one canonical economic event by UUID.
func (repository *Repository) EconomicEvent(ctx context.Context, id string) (calendar.StoredEvent, error) {
	if err := validateEconomicEventID(id); err != nil {
		return calendar.StoredEvent{}, err
	}

	event, err := scanEvent(repository.db.QueryRow(ctx, economicEventSQL, id))
	if err != nil {
		return calendar.StoredEvent{}, fmt.Errorf("query economic event: %w", err)
	}
	return event, nil
}

func validateEconomicEventID(id string) error {
	var value pgtype.UUID
	if err := value.Scan(id); err != nil || !value.Valid {
		return errors.New("event ID must be a UUID")
	}
	return nil
}

const economicEventSQL = `
SELECT ` + eventColumns + `
FROM economic_events
WHERE id = $1`
