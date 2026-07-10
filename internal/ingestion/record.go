package ingestion

import "time"

// SourceRecord is the normalized representation of one item from a source.
type SourceRecord struct {
	Source       string
	SourceItemID string
	OriginalURL  string
	Title        string
	PublishedAt  time.Time
	RetrievedAt  time.Time
}
