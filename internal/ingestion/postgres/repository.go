// Package postgres persists normalized ingestion records in PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/jackc/pgx/v5"
)

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Repository persists normalized source records.
type Repository struct {
	db DB
}

// StoredSourceRecord is a normalized source record with its persistence metadata.
type StoredSourceRecord struct {
	ID string
	ingestion.SourceRecord
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

// NewRepository returns a source-record repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}

	return &Repository{db: db}, nil
}

// UpsertSourceRecord inserts a source record or applies metadata from a newer retrieval.
// Source identity and creation audit fields remain immutable after the first insert.
func (repository *Repository) UpsertSourceRecord(
	ctx context.Context,
	record ingestion.SourceRecord,
	actor string,
) (StoredSourceRecord, error) {
	actor = strings.TrimSpace(actor)
	if err := validateSourceRecord(record, actor); err != nil {
		return StoredSourceRecord{}, err
	}

	record.PublishedAt = record.PublishedAt.UTC()
	record.RetrievedAt = record.RetrievedAt.UTC()

	stored, err := scanSourceRecord(repository.db.QueryRow(
		ctx,
		upsertSourceRecordSQL,
		record.Source,
		record.SourceItemID,
		record.OriginalURL,
		record.Title,
		record.PublishedAt,
		record.RetrievedAt,
		actor,
	))
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return StoredSourceRecord{}, fmt.Errorf("upsert source record: %w", err)
	}

	stored, err = scanSourceRecord(repository.db.QueryRow(
		ctx,
		selectSourceRecordSQL,
		record.Source,
		record.SourceItemID,
	))
	if err != nil {
		return StoredSourceRecord{}, fmt.Errorf("load unchanged source record: %w", err)
	}

	return stored, nil
}

func scanSourceRecord(row pgx.Row) (StoredSourceRecord, error) {
	var record StoredSourceRecord
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

func validateSourceRecord(record ingestion.SourceRecord, actor string) error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "source", value: record.Source},
		{name: "source item ID", value: record.SourceItemID},
		{name: "title", value: record.Title},
		{name: "actor", value: actor},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}

	parsedURL, err := url.Parse(record.OriginalURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Hostname() == "" {
		return errors.New("original URL must be an absolute HTTP(S) URL")
	}
	if record.PublishedAt.IsZero() {
		return errors.New("published time is required")
	}
	if record.RetrievedAt.IsZero() {
		return errors.New("retrieved time is required")
	}

	return nil
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

const upsertSourceRecordSQL = `
INSERT INTO source_records (
    source,
    source_item_id,
    original_url,
    title,
    published_at,
    retrieved_at,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
ON CONFLICT (source, source_item_id) DO UPDATE
SET original_url = EXCLUDED.original_url,
    title = EXCLUDED.title,
    published_at = EXCLUDED.published_at,
    retrieved_at = EXCLUDED.retrieved_at,
    updated_at = statement_timestamp(),
    updated_by = EXCLUDED.updated_by
WHERE EXCLUDED.retrieved_at > source_records.retrieved_at
RETURNING ` + sourceRecordColumns

const selectSourceRecordSQL = `
SELECT ` + sourceRecordColumns + `
FROM source_records
WHERE source = $1 AND source_item_id = $2`
