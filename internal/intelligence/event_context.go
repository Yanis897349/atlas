// Package intelligence assembles deterministic source-cited macro context.
package intelligence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5/pgtype"
)

// EventContextQuery selects one economic event and its related source-record window.
type EventContextQuery struct {
	EventID                string
	PublicationWindowStart time.Time
	PublicationWindowEnd   time.Time
	SourceRecordLimit      int
}

// EventContext contains one canonical event and its semantically related source records.
type EventContext struct {
	Event                  calendar.StoredEvent
	PublicationWindowStart time.Time
	PublicationWindowEnd   time.Time
	SourceRecords          []search.SimilarSourceRecord
}

// EconomicEventReader retrieves canonical economic events by UUID.
type EconomicEventReader interface {
	EconomicEvent(context.Context, string) (calendar.StoredEvent, error)
}

// AssembleEventContext loads one event and retrieves source records related to its exact persisted name.
func AssembleEventContext(
	ctx context.Context,
	events EconomicEventReader,
	embedder search.Embedder,
	sourceRecords search.SimilarSourceRecordReader,
	query EventContextQuery,
) (EventContext, error) {
	query, err := normalizeAndValidateEventContextQuery(query)
	if err != nil {
		return EventContext{}, fmt.Errorf("validate economic event context query: %w", err)
	}

	event, err := events.EconomicEvent(ctx, query.EventID)
	if err != nil {
		return EventContext{}, fmt.Errorf("retrieve economic event: %w", err)
	}

	filters := search.SimilarSourceRecordFilters{
		PublicationWindowStart: &query.PublicationWindowStart,
		PublicationWindowEnd:   &query.PublicationWindowEnd,
	}
	records, err := search.SearchSourceRecords(
		ctx,
		embedder,
		sourceRecords,
		event.Name,
		filters,
		query.SourceRecordLimit,
	)
	if err != nil {
		return EventContext{}, fmt.Errorf("retrieve economic event source records: %w", err)
	}

	return EventContext{
		Event:                  event,
		PublicationWindowStart: query.PublicationWindowStart,
		PublicationWindowEnd:   query.PublicationWindowEnd,
		SourceRecords:          records,
	}, nil
}

func normalizeAndValidateEventContextQuery(query EventContextQuery) (EventContextQuery, error) {
	var eventID pgtype.UUID
	if err := eventID.Scan(query.EventID); err != nil || !eventID.Valid {
		return EventContextQuery{}, errors.New("event ID must be a UUID")
	}
	if query.PublicationWindowStart.IsZero() {
		return EventContextQuery{}, errors.New("publication window start is required")
	}
	if query.PublicationWindowEnd.IsZero() {
		return EventContextQuery{}, errors.New("publication window end is required")
	}
	if query.PublicationWindowEnd.Before(query.PublicationWindowStart) {
		return EventContextQuery{}, errors.New("publication window end must not be before start")
	}
	if query.SourceRecordLimit < 1 || query.SourceRecordLimit > search.MaxSimilarSourceRecordsLimit {
		return EventContextQuery{}, fmt.Errorf(
			"source record limit must be between 1 and %d",
			search.MaxSimilarSourceRecordsLimit,
		)
	}

	query.EventID = eventID.String()
	query.PublicationWindowStart = query.PublicationWindowStart.UTC()
	query.PublicationWindowEnd = query.PublicationWindowEnd.UTC()
	return query, nil
}
