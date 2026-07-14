package search

import (
	"context"
	"errors"
	"strings"
	"time"

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

// SimilarSourceRecordFilters limits canonical source records considered for similarity retrieval.
type SimilarSourceRecordFilters struct {
	Source                 *string
	PublicationWindowStart *time.Time
	PublicationWindowEnd   *time.Time
}

// NormalizeAndValidateSimilarSourceRecordFilters returns a normalized copy of similarity filters.
func NormalizeAndValidateSimilarSourceRecordFilters(
	filters SimilarSourceRecordFilters,
) (SimilarSourceRecordFilters, error) {
	if filters.Source != nil {
		normalizedSource := strings.TrimSpace(*filters.Source)
		if normalizedSource == "" {
			return SimilarSourceRecordFilters{}, errors.New("source is required when supplied")
		}
		filters.Source = &normalizedSource
	}
	if (filters.PublicationWindowStart == nil) != (filters.PublicationWindowEnd == nil) {
		return SimilarSourceRecordFilters{}, errors.New("publication window start and end must be supplied together")
	}
	if filters.PublicationWindowStart == nil {
		return filters, nil
	}
	if filters.PublicationWindowStart.IsZero() {
		return SimilarSourceRecordFilters{}, errors.New("publication window start is required when supplied")
	}
	if filters.PublicationWindowEnd.IsZero() {
		return SimilarSourceRecordFilters{}, errors.New("publication window end is required when supplied")
	}
	if filters.PublicationWindowEnd.Before(*filters.PublicationWindowStart) {
		return SimilarSourceRecordFilters{}, errors.New("publication window end must not be before start")
	}
	normalizedStart := filters.PublicationWindowStart.UTC()
	normalizedEnd := filters.PublicationWindowEnd.UTC()
	filters.PublicationWindowStart = &normalizedStart
	filters.PublicationWindowEnd = &normalizedEnd
	return filters, nil
}

// SimilarSourceRecordReader retrieves canonical source records ranked by embedding distance.
type SimilarSourceRecordReader interface {
	SimilarSourceRecords(
		context.Context,
		string,
		string,
		[]float32,
		SimilarSourceRecordFilters,
		int,
	) ([]SimilarSourceRecord, error)
}
