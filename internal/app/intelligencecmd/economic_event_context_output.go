package intelligencecmd

import (
	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

type economicEventContextOutput struct {
	Event                  economicEventOutput                     `json:"event"`
	PublicationWindowStart string                                  `json:"publication_window_start"`
	PublicationWindowEnd   string                                  `json:"publication_window_end"`
	Observations           []economicEventContextObservationOutput `json:"observations"`
	SourceRecords          []economicEventSourceOutput             `json:"source_records"`
}

type economicEventOutput struct {
	ID              string             `json:"id"`
	Source          string             `json:"source"`
	ExternalEventID string             `json:"external_event_id"`
	Name            string             `json:"name"`
	Region          calendar.Region    `json:"region"`
	EventType       calendar.EventType `json:"event_type"`
	ScheduledAt     string             `json:"scheduled_at"`
	SourceURL       string             `json:"source_url"`
	RetrievedAt     string             `json:"retrieved_at"`
	CreatedAt       string             `json:"created_at"`
	UpdatedAt       string             `json:"updated_at"`
	CreatedBy       string             `json:"created_by"`
	UpdatedBy       string             `json:"updated_by"`
}

type economicEventSourceOutput struct {
	ID             string  `json:"id"`
	Source         string  `json:"source"`
	SourceItemID   string  `json:"source_item_id"`
	OriginalURL    string  `json:"original_url"`
	Title          string  `json:"title"`
	PublishedAt    string  `json:"published_at"`
	RetrievedAt    string  `json:"retrieved_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	CreatedBy      string  `json:"created_by"`
	UpdatedBy      string  `json:"updated_by"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	CosineDistance float64 `json:"cosine_distance"`
}

func newEconomicEventContextOutput(eventContext intelligence.EventContext) economicEventContextOutput {
	result := economicEventContextOutput{
		Event:                  newEconomicEventOutput(eventContext.Event),
		PublicationWindowStart: output.FormatTime(eventContext.PublicationWindowStart),
		PublicationWindowEnd:   output.FormatTime(eventContext.PublicationWindowEnd),
		Observations:           make([]economicEventContextObservationOutput, 0, len(eventContext.Observations)),
		SourceRecords:          make([]economicEventSourceOutput, 0, len(eventContext.SourceRecords)),
	}
	for _, observation := range eventContext.Observations {
		result.Observations = append(
			result.Observations,
			newEconomicEventContextObservationOutput(observation),
		)
	}
	for _, record := range eventContext.SourceRecords {
		result.SourceRecords = append(result.SourceRecords, newEconomicEventSourceOutput(record))
	}
	return result
}

func newEconomicEventOutput(event calendar.StoredEvent) economicEventOutput {
	return economicEventOutput{
		ID:              event.ID,
		Source:          event.Source,
		ExternalEventID: event.ExternalEventID,
		Name:            event.Name,
		Region:          event.Region,
		EventType:       event.Type,
		ScheduledAt:     output.FormatTime(event.ScheduledAt),
		SourceURL:       event.SourceURL,
		RetrievedAt:     output.FormatTime(event.RetrievedAt),
		CreatedAt:       output.FormatTime(event.CreatedAt),
		UpdatedAt:       output.FormatTime(event.UpdatedAt),
		CreatedBy:       event.CreatedBy,
		UpdatedBy:       event.UpdatedBy,
	}
}

func newEconomicEventSourceOutput(match search.SimilarSourceRecord) economicEventSourceOutput {
	record := match.SourceRecord
	return economicEventSourceOutput{
		ID:             record.ID,
		Source:         record.Source,
		SourceItemID:   record.SourceItemID,
		OriginalURL:    record.OriginalURL,
		Title:          record.Title,
		PublishedAt:    output.FormatTime(record.PublishedAt),
		RetrievedAt:    output.FormatTime(record.RetrievedAt),
		CreatedAt:      output.FormatTime(record.CreatedAt),
		UpdatedAt:      output.FormatTime(record.UpdatedAt),
		CreatedBy:      record.CreatedBy,
		UpdatedBy:      record.UpdatedBy,
		Provider:       match.Provider,
		Model:          match.Model,
		CosineDistance: match.CosineDistance,
	}
}
