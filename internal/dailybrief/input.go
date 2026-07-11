package dailybrief

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
)

// InputQuery selects the deterministic context used to create a daily brief.
type InputQuery struct {
	Region                 calendar.Region
	PublicationWindowStart time.Time
	PublicationWindowEnd   time.Time
	SourceRecordLimit      int
	EventWindowStart       time.Time
	EventWindowEnd         time.Time
	UpcomingEventLimit     int
}

// Input contains the source-cited context supplied to a generator.
type Input struct {
	Region                 calendar.Region
	PublicationWindowStart time.Time
	PublicationWindowEnd   time.Time
	EventWindowStart       time.Time
	EventWindowEnd         time.Time
	SourceRecords          []ingestion.StoredSourceRecord
	UpcomingEvents         []calendar.StoredEvent
}

// SourceRecords retrieves source records used to assemble an input.
type SourceRecords interface {
	RecentSourceRecords(context.Context, time.Time, time.Time, int) ([]ingestion.StoredSourceRecord, error)
}

// Events retrieves economic events used to assemble an input.
type Events interface {
	UpcomingEvents(context.Context, calendar.Region, time.Time, time.Time, int) ([]calendar.StoredEvent, error)
}

// AssembleInput retrieves and assembles deterministic daily-brief context.
func AssembleInput(
	ctx context.Context,
	sourceRecords SourceRecords,
	events Events,
	query InputQuery,
) (Input, error) {
	if err := validateInputQuery(query); err != nil {
		return Input{}, err
	}

	query.PublicationWindowStart = query.PublicationWindowStart.UTC()
	query.PublicationWindowEnd = query.PublicationWindowEnd.UTC()
	query.EventWindowStart = query.EventWindowStart.UTC()
	query.EventWindowEnd = query.EventWindowEnd.UTC()

	records, err := sourceRecords.RecentSourceRecords(
		ctx,
		query.PublicationWindowStart,
		query.PublicationWindowEnd,
		query.SourceRecordLimit,
	)
	if err != nil {
		return Input{}, fmt.Errorf("retrieve daily brief source records: %w", err)
	}

	upcomingEvents, err := events.UpcomingEvents(
		ctx,
		query.Region,
		query.EventWindowStart,
		query.EventWindowEnd,
		query.UpcomingEventLimit,
	)
	if err != nil {
		return Input{}, fmt.Errorf("retrieve daily brief upcoming events: %w", err)
	}

	return Input{
		Region:                 query.Region,
		PublicationWindowStart: query.PublicationWindowStart,
		PublicationWindowEnd:   query.PublicationWindowEnd,
		EventWindowStart:       query.EventWindowStart,
		EventWindowEnd:         query.EventWindowEnd,
		SourceRecords:          records,
		UpcomingEvents:         upcomingEvents,
	}, nil
}

func validateInputQuery(query InputQuery) error {
	if query.Region != calendar.RegionUnitedStates && query.Region != calendar.RegionEurozone {
		return fmt.Errorf("unsupported region %q", query.Region)
	}
	if query.PublicationWindowStart.IsZero() {
		return errors.New("publication window start is required")
	}
	if query.PublicationWindowEnd.IsZero() {
		return errors.New("publication window end is required")
	}
	if query.PublicationWindowEnd.Before(query.PublicationWindowStart) {
		return errors.New("publication window end must not be before publication window start")
	}
	if query.SourceRecordLimit < 1 || query.SourceRecordLimit > ingestion.MaxRecentSourceRecordsLimit {
		return fmt.Errorf(
			"source record limit must be between 1 and %d",
			ingestion.MaxRecentSourceRecordsLimit,
		)
	}
	if query.EventWindowStart.IsZero() {
		return errors.New("event window start is required")
	}
	if query.EventWindowEnd.IsZero() {
		return errors.New("event window end is required")
	}
	if query.EventWindowEnd.Before(query.EventWindowStart) {
		return errors.New("event window end must not be before event window start")
	}
	if query.UpcomingEventLimit < 1 || query.UpcomingEventLimit > calendar.MaxUpcomingEventsLimit {
		return fmt.Errorf(
			"upcoming event limit must be between 1 and %d",
			calendar.MaxUpcomingEventsLimit,
		)
	}

	return nil
}
