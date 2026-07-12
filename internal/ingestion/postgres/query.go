package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

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
	rows, err := repository.db.Query(ctx, recentSourceRecordsSQL, windowStart.UTC(), windowEnd.UTC(), limit)
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

func validateRecentSourceRecordsQuery(windowStart, windowEnd time.Time, limit int) error {
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

const recentSourceRecordsSQL = `
SELECT ` + sourceRecordColumns + `
FROM source_records
WHERE published_at >= $1
  AND published_at <= $2
ORDER BY published_at DESC, id
LIMIT $3`
