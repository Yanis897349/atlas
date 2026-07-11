// Package watchlist defines user-authored instrument watchlists.
package watchlist

import (
	"context"
	"time"
)

// MaxWatchlistsLimit bounds one watchlist list retrieval.
const MaxWatchlistsLimit = 100

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
