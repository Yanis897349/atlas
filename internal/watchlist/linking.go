package watchlist

import (
	"context"
	"fmt"

	"github.com/Yanis897349/atlas/internal/calendar"
)

// WatchlistReader retrieves persisted watchlist definitions for event linking.
type WatchlistReader interface {
	Watchlist(context.Context, string) (StoredWatchlist, error)
}

// EventLinkWriter atomically persists relevant watchlist event classifications.
type EventLinkWriter interface {
	CreateEventLinks(context.Context, string, []EventRelevance, string) ([]StoredEventLink, error)
}

// LinkRelevantEvents classifies supplied events for a persisted watchlist and stores relevant links.
func LinkRelevantEvents(
	ctx context.Context,
	reader WatchlistReader,
	writer EventLinkWriter,
	watchlistID string,
	events []calendar.StoredEvent,
	actor string,
) ([]StoredEventLink, error) {
	stored, err := reader.Watchlist(ctx, watchlistID)
	if err != nil {
		return nil, fmt.Errorf("retrieve watchlist for event linking: %w", err)
	}

	classifications, err := ClassifyWatchlistEvents(stored.Symbols, events)
	if err != nil {
		return nil, fmt.Errorf("classify watchlist events: %w", err)
	}

	links, err := writer.CreateEventLinks(ctx, stored.ID, classifications, actor)
	if err != nil {
		return nil, fmt.Errorf("persist classified watchlist event links: %w", err)
	}
	return links, nil
}
