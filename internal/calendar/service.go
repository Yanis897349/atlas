package calendar

import (
	"context"
	"fmt"
)

// Adapter retrieves normalized economic events from one calendar source.
type Adapter interface {
	FetchEvents(context.Context) ([]Event, error)
}

// Repository persists normalized economic events.
type Repository interface {
	PersistEvent(context.Context, Event, string) error
}

// Ingest fetches one calendar source and persists every normalized event.
func Ingest(ctx context.Context, adapter Adapter, repository Repository, actor string) (int, error) {
	events, err := adapter.FetchEvents(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch economic events: %w", err)
	}

	for index, event := range events {
		if err := repository.PersistEvent(ctx, event, actor); err != nil {
			return index, fmt.Errorf("persist economic event %d: %w", index+1, err)
		}
	}

	return len(events), nil
}
