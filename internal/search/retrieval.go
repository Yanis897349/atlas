package search

import (
	"context"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

// MaxSimilarSourceRecordsLimit bounds one similar-source-record retrieval.
const MaxSimilarSourceRecordsLimit = 100

// SimilarSourceRecord is one canonical source record ranked by embedding distance.
type SimilarSourceRecord struct {
	SourceRecord   ingestion.StoredSourceRecord
	Provider       string
	Model          string
	CosineDistance float64
}

// SimilarSourceRecordReader retrieves canonical source records ranked by embedding distance.
type SimilarSourceRecordReader interface {
	SimilarSourceRecords(context.Context, string, string, []float32, int) ([]SimilarSourceRecord, error)
}
