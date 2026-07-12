// Package record owns PostgreSQL row mapping for canonical source records.
package record

import (
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

// Row is a PostgreSQL row that can scan a canonical source record.
type Row interface {
	Scan(...any) error
}

// Columns returns the canonical source-record projection for qualifier.
func Columns(qualifier string) string {
	prefix := ""
	if qualifier = strings.TrimSpace(qualifier); qualifier != "" {
		prefix = qualifier + "."
	}
	return `
    ` + prefix + `id::text,
    ` + prefix + `source,
    ` + prefix + `source_item_id,
    ` + prefix + `original_url,
    ` + prefix + `title,
    ` + prefix + `published_at,
    ` + prefix + `retrieved_at,
    ` + prefix + `created_at,
    ` + prefix + `updated_at,
    ` + prefix + `created_by,
    ` + prefix + `updated_by`
}

// Scan maps row to one canonical stored source record without changing timestamp locations.
func Scan(row Row) (ingestion.StoredSourceRecord, error) {
	var stored ingestion.StoredSourceRecord
	err := row.Scan(Destinations(&stored)...)
	return stored, err
}

// Destinations returns canonical source-record scan destinations for use in joined projections.
func Destinations(stored *ingestion.StoredSourceRecord) []any {
	return []any{
		&stored.ID,
		&stored.Source,
		&stored.SourceItemID,
		&stored.OriginalURL,
		&stored.Title,
		&stored.PublishedAt,
		&stored.RetrievedAt,
		&stored.CreatedAt,
		&stored.UpdatedAt,
		&stored.CreatedBy,
		&stored.UpdatedBy,
	}
}

// NormalizeTimes converts all stored source-record timestamps to UTC without changing their instants.
func NormalizeTimes(stored *ingestion.StoredSourceRecord) {
	for _, value := range []*time.Time{
		&stored.PublishedAt,
		&stored.RetrievedAt,
		&stored.CreatedAt,
		&stored.UpdatedAt,
	} {
		*value = value.UTC()
	}
}
