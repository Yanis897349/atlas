// Package watchlist defines user-authored instrument watchlists.
package watchlist

import (
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
