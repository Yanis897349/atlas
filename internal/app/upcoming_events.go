package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
)

type upcomingEventsQuery struct {
	region      calendar.Region
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

type upcomingEventsRepository interface {
	UpcomingEvents(context.Context, calendar.Region, time.Time, time.Time, int) ([]calendarpostgres.StoredEvent, error)
}

type upcomingEventOutput struct {
	ID              string             `json:"id"`
	Source          string             `json:"source"`
	ExternalEventID string             `json:"external_event_id"`
	Name            string             `json:"name"`
	Region          calendar.Region    `json:"region"`
	EventType       calendar.EventType `json:"event_type"`
	ScheduledAt     string             `json:"scheduled_at"`
	SourceURL       string             `json:"source_url"`
	RetrievedAt     string             `json:"retrieved_at"`
}

func runUpcomingEvents(
	ctx context.Context,
	repository upcomingEventsRepository,
	stdout io.Writer,
	query upcomingEventsQuery,
) error {
	events, err := repository.UpcomingEvents(
		ctx,
		query.region,
		query.windowStart,
		query.windowEnd,
		query.limit,
	)
	if err != nil {
		return fmt.Errorf("list upcoming economic events: %w", err)
	}

	output := make([]upcomingEventOutput, 0, len(events))
	for _, event := range events {
		output = append(output, newUpcomingEventOutput(event))
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("encode upcoming economic events: %w", err)
	}
	return nil
}

func newUpcomingEventOutput(event calendarpostgres.StoredEvent) upcomingEventOutput {
	return upcomingEventOutput{
		ID:              event.ID,
		Source:          event.Source,
		ExternalEventID: event.ExternalEventID,
		Name:            event.Name,
		Region:          event.Region,
		EventType:       event.Type,
		ScheduledAt:     formatOutputTime(event.ScheduledAt),
		SourceURL:       event.SourceURL,
		RetrievedAt:     formatOutputTime(event.RetrievedAt),
	}
}

func formatOutputTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
