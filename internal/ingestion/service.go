package ingestion

import (
	"context"
	"fmt"
)

// Fetcher retrieves normalized source records from one source.
type Fetcher interface {
	Fetch(context.Context) ([]SourceRecord, error)
}

// Repository upserts normalized source records and returns their canonical stored form.
type Repository interface {
	UpsertSourceRecord(context.Context, SourceRecord, string) (StoredSourceRecord, error)
}

// Ingest fetches one source and returns every canonical stored record in fetch order.
func Ingest(
	ctx context.Context,
	fetcher Fetcher,
	repository Repository,
	actor string,
) ([]StoredSourceRecord, error) {
	records, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch source records: %w", err)
	}

	storedRecords := make([]StoredSourceRecord, 0, len(records))
	for index, record := range records {
		stored, err := repository.UpsertSourceRecord(ctx, record, actor)
		if err != nil {
			return storedRecords, fmt.Errorf("persist source record %d: %w", index+1, err)
		}
		storedRecords = append(storedRecords, stored)
	}

	return storedRecords, nil
}
