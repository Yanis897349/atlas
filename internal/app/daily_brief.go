package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

type dailyBriefInputQuery struct {
	region                 calendar.Region
	publicationWindowStart time.Time
	publicationWindowEnd   time.Time
	sourceRecordLimit      int
	eventWindowStart       time.Time
	eventWindowEnd         time.Time
	upcomingEventLimit     int
}

type dailyBriefInput struct {
	region                 calendar.Region
	publicationWindowStart time.Time
	publicationWindowEnd   time.Time
	eventWindowStart       time.Time
	eventWindowEnd         time.Time
	sourceRecords          []ingestionpostgres.StoredSourceRecord
	upcomingEvents         []calendarpostgres.StoredEvent
}

type recentSourceRecordsRepository interface {
	RecentSourceRecords(context.Context, time.Time, time.Time, int) ([]ingestionpostgres.StoredSourceRecord, error)
}

type dailyBriefEventsRepository interface {
	UpcomingEvents(context.Context, calendar.Region, time.Time, time.Time, int) ([]calendarpostgres.StoredEvent, error)
}

func assembleDailyBriefInput(
	ctx context.Context,
	sourceRecords recentSourceRecordsRepository,
	events dailyBriefEventsRepository,
	query dailyBriefInputQuery,
) (dailyBriefInput, error) {
	if err := validateDailyBriefInputQuery(query); err != nil {
		return dailyBriefInput{}, err
	}

	query.publicationWindowStart = query.publicationWindowStart.UTC()
	query.publicationWindowEnd = query.publicationWindowEnd.UTC()
	query.eventWindowStart = query.eventWindowStart.UTC()
	query.eventWindowEnd = query.eventWindowEnd.UTC()

	records, err := sourceRecords.RecentSourceRecords(
		ctx,
		query.publicationWindowStart,
		query.publicationWindowEnd,
		query.sourceRecordLimit,
	)
	if err != nil {
		return dailyBriefInput{}, fmt.Errorf("retrieve daily brief source records: %w", err)
	}

	upcomingEvents, err := events.UpcomingEvents(
		ctx,
		query.region,
		query.eventWindowStart,
		query.eventWindowEnd,
		query.upcomingEventLimit,
	)
	if err != nil {
		return dailyBriefInput{}, fmt.Errorf("retrieve daily brief upcoming events: %w", err)
	}

	return dailyBriefInput{
		region:                 query.region,
		publicationWindowStart: query.publicationWindowStart,
		publicationWindowEnd:   query.publicationWindowEnd,
		eventWindowStart:       query.eventWindowStart,
		eventWindowEnd:         query.eventWindowEnd,
		sourceRecords:          records,
		upcomingEvents:         upcomingEvents,
	}, nil
}

func validateDailyBriefInputQuery(query dailyBriefInputQuery) error {
	if query.region != calendar.RegionUnitedStates && query.region != calendar.RegionEurozone {
		return fmt.Errorf("unsupported region %q", query.region)
	}
	if query.publicationWindowStart.IsZero() {
		return errors.New("publication window start is required")
	}
	if query.publicationWindowEnd.IsZero() {
		return errors.New("publication window end is required")
	}
	if query.publicationWindowEnd.Before(query.publicationWindowStart) {
		return errors.New("publication window end must not be before publication window start")
	}
	if query.sourceRecordLimit < 1 || query.sourceRecordLimit > ingestionpostgres.MaxRecentSourceRecordsLimit {
		return fmt.Errorf(
			"source record limit must be between 1 and %d",
			ingestionpostgres.MaxRecentSourceRecordsLimit,
		)
	}
	if query.eventWindowStart.IsZero() {
		return errors.New("event window start is required")
	}
	if query.eventWindowEnd.IsZero() {
		return errors.New("event window end is required")
	}
	if query.eventWindowEnd.Before(query.eventWindowStart) {
		return errors.New("event window end must not be before event window start")
	}
	if query.upcomingEventLimit < 1 || query.upcomingEventLimit > calendarpostgres.MaxUpcomingEventsLimit {
		return fmt.Errorf(
			"upcoming event limit must be between 1 and %d",
			calendarpostgres.MaxUpcomingEventsLimit,
		)
	}

	return nil
}
