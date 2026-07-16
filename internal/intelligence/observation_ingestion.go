package intelligence

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// MaxObservationIngestionLimit bounds one economic-event observation ingestion.
const MaxObservationIngestionLimit = 100

// ObservationAdapter retrieves a bounded set of normalized economic-event observations from one source.
type ObservationAdapter interface {
	FetchObservations(context.Context, int) ([]Observation, error)
}

// IngestObservations fetches and persists observations in adapter order.
func IngestObservations(
	ctx context.Context,
	adapter ObservationAdapter,
	writer ObservationWriter,
	limit int,
	actor string,
) (int, error) {
	if limit < 1 || limit > MaxObservationIngestionLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", MaxObservationIngestionLimit)
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return 0, errors.New("actor is required")
	}

	observations, err := adapter.FetchObservations(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("fetch economic event observations: %w", err)
	}
	if len(observations) > limit {
		return 0, fmt.Errorf(
			"validate fetched economic event observations: adapter returned %d observations for limit %d",
			len(observations),
			limit,
		)
	}

	for index, observation := range observations {
		if _, err := writer.StoreObservation(ctx, observation, actor); err != nil {
			return index, fmt.Errorf("persist economic event observation %d: %w", index+1, err)
		}
	}
	return len(observations), nil
}
