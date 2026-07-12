// Package postgres persists normalized ingestion records in PostgreSQL.
package postgres

import (
	"context"
	"errors"

	"github.com/Yanis897349/atlas/internal/ingestion"
	recordpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres/record"
	"github.com/jackc/pgx/v5"
)

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Repository persists normalized source records.
type Repository struct {
	db DB
}

// NewRepository returns a source-record repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}

	return &Repository{db: db}, nil
}

func scanSourceRecord(row pgx.Row) (ingestion.StoredSourceRecord, error) {
	return recordpostgres.Scan(row)
}

var sourceRecordColumns = recordpostgres.Columns("")
