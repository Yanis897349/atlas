// Package postgres persists source-record embeddings in PostgreSQL.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Repository persists source-record embeddings and retrieves similar source records.
type Repository struct {
	db DB
}

// NewRepository returns a source-record embedding repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}
	return &Repository{db: db}, nil
}
