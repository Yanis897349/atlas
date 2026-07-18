package intelligencecmd

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

type economicEventContextOutput struct {
	Event                  economicEventOutput              `json:"event"`
	PublicationWindowStart string                           `json:"publication_window_start"`
	PublicationWindowEnd   string                           `json:"publication_window_end"`
	Observations           []economicEventObservationOutput `json:"observations"`
	SourceRecords          []economicEventSourceOutput      `json:"source_records"`
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

func runEconomicEventContext(
	ctx context.Context,
	events intelligence.EconomicEventReader,
	observations intelligence.ObservationReader,
	embedder search.Embedder,
	sourceRecords search.SimilarSourceRecordReader,
	stdout io.Writer,
	query intelligence.EventContextQuery,
) error {
	assembled, err := intelligence.AssembleEventContext(
		ctx,
		events,
		observations,
		embedder,
		sourceRecords,
		query,
	)
	if err != nil {
		return fmt.Errorf("assemble economic event context: %w", err)
	}

	result := economicEventContextOutput{
		Event:                  newEconomicEventOutput(assembled.Event),
		PublicationWindowStart: output.FormatTime(assembled.PublicationWindowStart),
		PublicationWindowEnd:   output.FormatTime(assembled.PublicationWindowEnd),
		Observations:           make([]economicEventObservationOutput, 0, len(assembled.Observations)),
		SourceRecords:          make([]economicEventSourceOutput, 0, len(assembled.SourceRecords)),
	}
	for _, observation := range assembled.Observations {
		result.Observations = append(result.Observations, newEconomicEventObservationOutput(observation))
	}
	for _, match := range assembled.SourceRecords {
		record := match.SourceRecord
		result.SourceRecords = append(result.SourceRecords, economicEventSourceOutput{
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
		})
	}

	var encoded bytes.Buffer
	if err := output.EncodeJSON(&encoded, "economic event context", result); err != nil {
		return err
	}
	written, err := stdout.Write(encoded.Bytes())
	if err != nil {
		return fmt.Errorf("write economic event context: %w", err)
	}
	if written != encoded.Len() {
		return fmt.Errorf("write economic event context: %w", io.ErrShortWrite)
	}
	return nil
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
