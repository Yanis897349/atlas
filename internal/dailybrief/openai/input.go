package openai

import (
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	"github.com/Yanis897349/atlas/internal/ingestion"
)

type openAIDailyBriefInput struct {
	Region            calendar.Region                 `json:"region"`
	PublicationWindow openAIDailyBriefWindow          `json:"publication_window"`
	EventWindow       openAIDailyBriefWindow          `json:"event_window"`
	SourceRecords     []openAIDailyBriefSourceRecord  `json:"source_records"`
	UpcomingEvents    []openAIDailyBriefUpcomingEvent `json:"upcoming_events"`
}

type openAIDailyBriefWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type openAIDailyBriefSourceRecord struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceItemID string `json:"source_item_id"`
	OriginalURL  string `json:"original_url"`
	Title        string `json:"title"`
	PublishedAt  string `json:"published_at"`
	RetrievedAt  string `json:"retrieved_at"`
}

type openAIDailyBriefUpcomingEvent struct {
	ID              string             `json:"id"`
	Source          string             `json:"source"`
	ExternalEventID string             `json:"external_event_id"`
	Name            string             `json:"name"`
	Region          calendar.Region    `json:"region"`
	Type            calendar.EventType `json:"type"`
	ScheduledAt     string             `json:"scheduled_at"`
	SourceURL       string             `json:"source_url"`
	RetrievedAt     string             `json:"retrieved_at"`
}

func validateOpenAIDailyBriefInputSize(input dailybrief.Input) error {
	if len(input.SourceRecords) > ingestion.MaxRecentSourceRecordsLimit {
		return fmt.Errorf("daily brief input has more than %d source records", ingestion.MaxRecentSourceRecordsLimit)
	}
	if len(input.UpcomingEvents) > calendar.MaxUpcomingEventsLimit {
		return fmt.Errorf("daily brief input has more than %d upcoming events", calendar.MaxUpcomingEventsLimit)
	}

	remaining := maxOpenAIDailyBriefInputBytes
	add := func(value string) error {
		if len(value) > remaining {
			return errors.New("daily brief input is too large")
		}
		remaining -= len(value)
		return nil
	}

	if err := add(string(input.Region)); err != nil {
		return err
	}
	for _, record := range input.SourceRecords {
		for _, value := range []string{record.ID, record.Source, record.SourceItemID, record.OriginalURL, record.Title} {
			if err := add(value); err != nil {
				return err
			}
		}
	}
	for _, event := range input.UpcomingEvents {
		for _, value := range []string{
			event.ID, event.Source, event.ExternalEventID, event.Name,
			string(event.Region), string(event.Type), event.SourceURL,
		} {
			if err := add(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func newOpenAIDailyBriefInput(input dailybrief.Input) openAIDailyBriefInput {
	providerInput := openAIDailyBriefInput{
		Region: input.Region,
		PublicationWindow: openAIDailyBriefWindow{
			From: formatOpenAITime(input.PublicationWindowStart),
			To:   formatOpenAITime(input.PublicationWindowEnd),
		},
		EventWindow: openAIDailyBriefWindow{
			From: formatOpenAITime(input.EventWindowStart),
			To:   formatOpenAITime(input.EventWindowEnd),
		},
		SourceRecords:  make([]openAIDailyBriefSourceRecord, 0, len(input.SourceRecords)),
		UpcomingEvents: make([]openAIDailyBriefUpcomingEvent, 0, len(input.UpcomingEvents)),
	}
	for _, record := range input.SourceRecords {
		providerInput.SourceRecords = append(providerInput.SourceRecords, openAIDailyBriefSourceRecord{
			ID: record.ID, Source: record.Source, SourceItemID: record.SourceItemID,
			OriginalURL: record.OriginalURL, Title: record.Title,
			PublishedAt: formatOpenAITime(record.PublishedAt), RetrievedAt: formatOpenAITime(record.RetrievedAt),
		})
	}
	for _, event := range input.UpcomingEvents {
		providerInput.UpcomingEvents = append(providerInput.UpcomingEvents, openAIDailyBriefUpcomingEvent{
			ID: event.ID, Source: event.Source, ExternalEventID: event.ExternalEventID, Name: event.Name,
			Region: event.Region, Type: event.Type, ScheduledAt: formatOpenAITime(event.ScheduledAt),
			SourceURL: event.SourceURL, RetrievedAt: formatOpenAITime(event.RetrievedAt),
		})
	}
	return providerInput
}

func formatOpenAITime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
