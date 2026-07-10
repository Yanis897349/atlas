package ingestion

import (
	"context"
	"fmt"
)

// Fetcher retrieves normalized source records from one source.
type Fetcher interface {
	Fetch(context.Context) ([]SourceRecord, error)
}

// Repository persists normalized source records.
type Repository interface {
	PersistSourceRecord(context.Context, SourceRecord, string) error
}

// Ingest fetches one source and persists every normalized record.
func Ingest(ctx context.Context, fetcher Fetcher, repository Repository, actor string) (int, error) {
	records, err := fetcher.Fetch(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch source records: %w", err)
	}

	for index, record := range records {
		if err := repository.PersistSourceRecord(ctx, record, actor); err != nil {
			return index, fmt.Errorf("persist source record %d: %w", index+1, err)
		}
	}

	return len(records), nil
}
