// Package postgres persists normalized ingestion records in PostgreSQL.
package postgres

import (
	"context"
	"errors"

	"github.com/Yanis897349/atlas/internal/ingestion"
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
	var record ingestion.StoredSourceRecord
	err := row.Scan(
		&record.ID,
		&record.Source,
		&record.SourceItemID,
		&record.OriginalURL,
		&record.Title,
		&record.PublishedAt,
		&record.RetrievedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.CreatedBy,
		&record.UpdatedBy,
	)
	return record, err
}

const sourceRecordColumns = `
    id::text,
    source,
    source_item_id,
    original_url,
    title,
    published_at,
    retrieved_at,
    created_at,
    updated_at,
    created_by,
    updated_by`
