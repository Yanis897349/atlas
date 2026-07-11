// Package watchlist defines user-authored instrument watchlists.
package watchlist

import (
	"context"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

// MaxWatchlistsLimit bounds one watchlist list retrieval.
const MaxWatchlistsLimit = 100

// MaxEventLinksLimit bounds one watchlist-instrument event-link retrieval.
const MaxEventLinksLimit = 100

// Definition is a user-authored watchlist before persistence.
type Definition struct {
	Name    string
	Symbols []string
}

// StoredWatchlist is a watchlist with persistence identity and audit metadata.
type StoredWatchlist struct {
	ID string
	Definition
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

// StoredEventLink associates one watchlist instrument with a canonical economic event.
type StoredEventLink struct {
	ID          string
	WatchlistID string
	Symbol      string
	Event       calendar.StoredEvent
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedBy   string
	UpdatedBy   string
}

// Persistence creates, updates, and deletes watchlist definitions.
type Persistence interface {
	CreateWatchlist(context.Context, Definition, string) (StoredWatchlist, error)
	UpdateWatchlist(context.Context, string, Definition, string) (StoredWatchlist, error)
	DeleteWatchlist(context.Context, string) error
}

// Reader retrieves stored watchlist definitions.
type Reader interface {
	Watchlist(context.Context, string) (StoredWatchlist, error)
	Watchlists(context.Context, int) ([]StoredWatchlist, error)
}

// EventLinkPersistence creates and deletes explicit watchlist-instrument event links.
type EventLinkPersistence interface {
	CreateEventLink(context.Context, string, string, string, string) (StoredEventLink, error)
	DeleteEventLink(context.Context, string, string, string) error
}

// EventLinkReader retrieves linked events for one watchlist instrument.
type EventLinkReader interface {
	EventLinks(context.Context, string, string, int) ([]StoredEventLink, error)
}
