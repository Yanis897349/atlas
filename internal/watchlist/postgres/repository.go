// Package postgres persists watchlist definitions in PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
)

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Repository persists and retrieves watchlist definitions.
type Repository struct {
	db DB
}

var (
	_ watchlist.Persistence = (*Repository)(nil)
	_ watchlist.Reader      = (*Repository)(nil)
)

// NewRepository returns a watchlist repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}
	return &Repository{db: db}, nil
}

// CreateWatchlist atomically creates an immutable watchlist definition.
func (repository *Repository) CreateWatchlist(
	ctx context.Context,
	definition watchlist.Definition,
	actor string,
) (watchlist.StoredWatchlist, error) {
	definition, actor, err := normalizeAndValidateDefinition(definition, actor)
	if err != nil {
		return watchlist.StoredWatchlist{}, err
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("begin watchlist creation: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	stored := watchlist.StoredWatchlist{Definition: definition}
	if err := transaction.QueryRow(ctx, insertWatchlistSQL, definition.Name, actor).Scan(
		&stored.ID,
		&stored.CreatedAt,
		&stored.UpdatedAt,
		&stored.CreatedBy,
		&stored.UpdatedBy,
	); err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("insert watchlist: %w", err)
	}
	stored.CreatedAt = stored.CreatedAt.UTC()
	stored.UpdatedAt = stored.UpdatedAt.UTC()

	for position, symbol := range definition.Symbols {
		var instrumentID string
		if err := transaction.QueryRow(
			ctx,
			insertWatchlistInstrumentSQL,
			stored.ID,
			position,
			symbol,
			actor,
		).Scan(&instrumentID); err != nil {
			return watchlist.StoredWatchlist{}, fmt.Errorf("insert watchlist instrument %d: %w", position, err)
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("commit watchlist creation: %w", err)
	}
	return stored, nil
}

const insertWatchlistSQL = `
INSERT INTO watchlists (name, created_by, updated_by)
VALUES ($1, $2, $2)
RETURNING id::text, created_at, updated_at, created_by, updated_by`

const insertWatchlistInstrumentSQL = `
INSERT INTO watchlist_instruments (
    watchlist_id,
    position,
    symbol,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $4)
RETURNING id::text`
