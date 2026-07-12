package watchlist

import (
	"context"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

// EventCandidateReader retrieves canonical economic events considered for watchlist linking.
type EventCandidateReader interface {
	WatchlistEventCandidates(context.Context, time.Time, time.Time, int) ([]calendar.StoredEvent, error)
}

// WatchlistReader retrieves persisted watchlist definitions for event linking.
type WatchlistReader interface {
	Watchlist(context.Context, string) (StoredWatchlist, error)
}

// EventLinkWriter atomically persists relevant watchlist event classifications.
type EventLinkWriter interface {
	CreateEventLinks(context.Context, string, []EventRelevance, string) ([]StoredEventLink, error)
}

// LinkRelevantEventCandidates retrieves candidate events and links those relevant to a persisted watchlist.
func LinkRelevantEventCandidates(
	ctx context.Context,
	candidates EventCandidateReader,
	reader WatchlistReader,
	writer EventLinkWriter,
	watchlistID string,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
	actor string,
) ([]StoredEventLink, error) {
	events, err := candidates.WatchlistEventCandidates(ctx, windowStart, windowEnd, limit)
	if err != nil {
		return nil, fmt.Errorf("retrieve watchlist event candidates: %w", err)
	}

	links, err := LinkRelevantEvents(ctx, reader, writer, watchlistID, events, actor)
	if err != nil {
		return nil, fmt.Errorf("link retrieved watchlist event candidates: %w", err)
	}
	return links, nil
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
