package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Yanis897349/atlas/internal/calendar"
	atlasuuid "github.com/Yanis897349/atlas/internal/uuid"
)

// EconomicEvent returns one canonical economic event by UUID.
func (repository *Repository) EconomicEvent(ctx context.Context, id string) (calendar.StoredEvent, error) {
	id, err := normalizeAndValidateEconomicEventID(id)
	if err != nil {
		return calendar.StoredEvent{}, err
	}

	event, err := scanEvent(repository.db.QueryRow(ctx, economicEventSQL, id))
	if err != nil {
		return calendar.StoredEvent{}, fmt.Errorf("query economic event: %w", err)
	}
	return event, nil
}

func normalizeAndValidateEconomicEventID(id string) (string, error) {
	normalized, valid := atlasuuid.Normalize(id)
	if !valid {
		return "", errors.New("event ID must be a UUID")
	}
	return normalized, nil
}

const economicEventSQL = `
SELECT ` + eventColumns + `
FROM economic_events
WHERE id = $1`
