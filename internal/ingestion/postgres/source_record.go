package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/jackc/pgx/v5"
)

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

	stored, err := scanSourceRecord(repository.db.QueryRow(ctx, upsertSourceRecordSQL, record.Source,
		record.SourceItemID, record.OriginalURL, record.Title, record.PublishedAt, record.RetrievedAt, actor))
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ingestion.StoredSourceRecord{}, fmt.Errorf("upsert source record: %w", err)
	}
	stored, err = scanSourceRecord(repository.db.QueryRow(ctx, selectSourceRecordSQL, record.Source, record.SourceItemID))
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

func validateSourceRecord(record ingestion.SourceRecord, actor string) error {
	for _, field := range []struct{ name, value string }{
		{name: "source", value: record.Source}, {name: "source item ID", value: record.SourceItemID},
		{name: "title", value: record.Title}, {name: "actor", value: actor},
	} {
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

var upsertSourceRecordSQL = `
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

var selectSourceRecordSQL = `
SELECT ` + sourceRecordColumns + `
FROM source_records
WHERE source = $1 AND source_item_id = $2`
