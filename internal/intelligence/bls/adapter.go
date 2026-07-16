// Package bls retrieves and normalizes official Bureau of Labor Statistics observations.
package bls

import (
	"context"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

// Adapter retrieves normalized BLS observation snapshots.
type Adapter struct {
	targets       []Target
	endpoint      string
	client        HTTPClient
	now           func() time.Time
	requestBudget time.Duration
}

var _ intelligence.ObservationAdapter = (*Adapter)(nil)

// FetchObservations retrieves up to limit snapshots in configured target order.
func (adapter *Adapter) FetchObservations(
	ctx context.Context,
	limit int,
) ([]intelligence.Observation, error) {
	if limit < 1 || limit > intelligence.MaxObservationIngestionLimit {
		return nil, fmt.Errorf(
			"limit must be between 1 and %d",
			intelligence.MaxObservationIngestionLimit,
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	count := min(limit, len(adapter.targets))
	targets := adapter.targets[:count]
	observedAt := adapter.now().UTC()
	body, err := adapter.fetch(ctx, targets, observedAt.Year()-2, observedAt.Year())
	if err != nil {
		return nil, err
	}
	observations, err := normalizeResponse(body, targets, observedAt)
	if err != nil {
		return nil, fmt.Errorf("normalize BLS observations: %w", err)
	}
	return observations, nil
}
