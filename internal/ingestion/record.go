package ingestion

import "time"

// MaxRecentSourceRecordsLimit bounds one recent-source-record retrieval.
const MaxRecentSourceRecordsLimit = 100

// SourceRecord is the normalized representation of one item from a source.
type SourceRecord struct {
	Source       string
	SourceItemID string
	OriginalURL  string
	Title        string
	PublishedAt  time.Time
	RetrievedAt  time.Time
}

// StoredSourceRecord is a normalized source record with its persistence metadata.
type StoredSourceRecord struct {
	ID string
	SourceRecord
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}
