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

// CreateWatchlist atomically creates a watchlist definition.
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

	if err := insertWatchlistInstruments(ctx, transaction, stored.ID, definition.Symbols, actor); err != nil {
		return watchlist.StoredWatchlist{}, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("commit watchlist creation: %w", err)
	}
	return stored, nil
}

// UpdateWatchlist atomically replaces a stored watchlist definition.
func (repository *Repository) UpdateWatchlist(
	ctx context.Context,
	id string,
	definition watchlist.Definition,
	actor string,
) (watchlist.StoredWatchlist, error) {
	if err := validateWatchlistID(id); err != nil {
		return watchlist.StoredWatchlist{}, err
	}
	definition, actor, err := normalizeAndValidateDefinition(definition, actor)
	if err != nil {
		return watchlist.StoredWatchlist{}, err
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("begin watchlist update: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	stored := watchlist.StoredWatchlist{Definition: definition}
	if err := transaction.QueryRow(ctx, updateWatchlistSQL, id, definition.Name, actor).Scan(
		&stored.ID,
		&stored.CreatedAt,
		&stored.UpdatedAt,
		&stored.CreatedBy,
		&stored.UpdatedBy,
	); err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("update watchlist: %w", err)
	}
	stored.CreatedAt = stored.CreatedAt.UTC()
	stored.UpdatedAt = stored.UpdatedAt.UTC()

	if _, err := transaction.Exec(ctx, deleteWatchlistInstrumentsSQL, stored.ID); err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("delete watchlist instruments: %w", err)
	}
	if err := insertWatchlistInstruments(ctx, transaction, stored.ID, definition.Symbols, actor); err != nil {
		return watchlist.StoredWatchlist{}, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return watchlist.StoredWatchlist{}, fmt.Errorf("commit watchlist update: %w", err)
	}
	return stored, nil
}

func insertWatchlistInstruments(
	ctx context.Context,
	transaction pgx.Tx,
	watchlistID string,
	symbols []string,
	actor string,
) error {
	for position, symbol := range symbols {
		var instrumentID string
		if err := transaction.QueryRow(
			ctx,
			insertWatchlistInstrumentSQL,
			watchlistID,
			position,
			symbol,
			actor,
		).Scan(&instrumentID); err != nil {
			return fmt.Errorf("insert watchlist instrument %d: %w", position, err)
		}
	}
	return nil
}

const insertWatchlistSQL = `
INSERT INTO watchlists (name, created_by, updated_by)
VALUES ($1, $2, $2)
RETURNING id::text, created_at, updated_at, created_by, updated_by`

const updateWatchlistSQL = `
UPDATE watchlists
SET name = $2,
    updated_at = statement_timestamp(),
    updated_by = $3
WHERE id = $1
RETURNING id::text, created_at, updated_at, created_by, updated_by`

const deleteWatchlistInstrumentsSQL = `
DELETE FROM watchlist_instruments
WHERE watchlist_id = $1`

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
