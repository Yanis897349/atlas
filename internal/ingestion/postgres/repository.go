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

// UpsertSourceRecord inserts a source record or applies metadata from a newer retrieval.
// Source identity and creation audit fields remain immutable after the first insert.
func (repository *Repository) UpsertSourceRecord(
	ctx context.Context,
	record ingestion.SourceRecord,
	actor string,
) (ingestion.StoredSourceRecord, error) {
	actor = strings.TrimSpace(actor)
	if err := validateSourceRecord(record, actor); err != nil {
		return ingestion.StoredSourceRecord{}, err
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
		return ingestion.StoredSourceRecord{}, fmt.Errorf("upsert source record: %w", err)
	}

	stored, err = scanSourceRecord(repository.db.QueryRow(
		ctx,
		selectSourceRecordSQL,
		record.Source,
		record.SourceItemID,
	))
	if err != nil {
		return ingestion.StoredSourceRecord{}, fmt.Errorf("load unchanged source record: %w", err)
	}

	return stored, nil
}

// PersistSourceRecord stores a source record for the ingestion service.
func (repository *Repository) PersistSourceRecord(ctx context.Context, record ingestion.SourceRecord, actor string) error {
	_, err := repository.UpsertSourceRecord(ctx, record, actor)
	return err
}

// RecentSourceRecords returns records published within the inclusive time window.
func (repository *Repository) RecentSourceRecords(
	ctx context.Context,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]ingestion.StoredSourceRecord, error) {
	if err := validateRecentSourceRecordsQuery(windowStart, windowEnd, limit); err != nil {
		return nil, err
	}

	rows, err := repository.db.Query(
		ctx,
		recentSourceRecordsSQL,
		windowStart.UTC(),
		windowEnd.UTC(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent source records: %w", err)
	}
	defer rows.Close()

	records := make([]ingestion.StoredSourceRecord, 0, limit)
	for rows.Next() {
		record, scanErr := scanSourceRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan recent source record: %w", scanErr)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent source records: %w", err)
	}

	return records, nil
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

func validateRecentSourceRecordsQuery(windowStart time.Time, windowEnd time.Time, limit int) error {
	if windowStart.IsZero() {
		return errors.New("window start is required")
	}
	if windowEnd.IsZero() {
		return errors.New("window end is required")
	}
	if windowEnd.Before(windowStart) {
		return errors.New("window end must not be before window start")
	}
	if limit < 1 || limit > ingestion.MaxRecentSourceRecordsLimit {
		return fmt.Errorf("limit must be between 1 and %d", ingestion.MaxRecentSourceRecordsLimit)
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

const recentSourceRecordsSQL = `
SELECT ` + sourceRecordColumns + `
FROM source_records
WHERE published_at >= $1
  AND published_at <= $2
ORDER BY published_at DESC, id
LIMIT $3`
