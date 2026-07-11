package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	"github.com/Yanis897349/atlas/internal/ingestion"
)

const openAIDailyBriefInstructions = `Create a concise macro daily brief using only the supplied source records and upcoming events. Explain context without predicting markets. Every section must cite at least one supplied item using its exact citation kind and ID. Do not invent facts, identifiers, sources, or URLs.`

var openAIDailyBriefJSONSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "sections": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "properties": {
          "heading": {"type": "string"},
          "content": {"type": "string"},
          "citations": {
            "type": "array",
            "minItems": 1,
            "items": {
              "type": "object",
              "properties": {
                "kind": {"type": "string", "enum": ["source_record", "upcoming_event"]},
                "id": {"type": "string"}
              },
              "required": ["kind", "id"],
              "additionalProperties": false
            }
          }
        },
        "required": ["heading", "content", "citations"],
        "additionalProperties": false
      }
    }
  },
  "required": ["sections"],
  "additionalProperties": false
}`)

type openAIResponseFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type openAIResponseText struct {
	Format openAIResponseFormat `json:"format"`
}

type openAIDailyBriefRequest struct {
	Model           string             `json:"model"`
	Instructions    string             `json:"instructions"`
	Input           string             `json:"input"`
	Text            openAIResponseText `json:"text"`
	MaxOutputTokens int                `json:"max_output_tokens"`
	Store           bool               `json:"store"`
}

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

func newOpenAIDailyBriefRequest(ctx context.Context, model string, input dailybrief.Input) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateOpenAIDailyBriefInputSize(input); err != nil {
		return nil, err
	}

	providerInput := newOpenAIDailyBriefInput(input)
	inputJSON, err := json.Marshal(providerInput)
	if err != nil {
		return nil, fmt.Errorf("encode daily brief input: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	request, err := json.Marshal(openAIDailyBriefRequest{
		Model:        model,
		Instructions: openAIDailyBriefInstructions,
		Input:        string(inputJSON),
		Text: openAIResponseText{Format: openAIResponseFormat{
			Type:   "json_schema",
			Name:   "daily_brief",
			Schema: openAIDailyBriefJSONSchema,
			Strict: true,
		}},
		MaxOutputTokens: maxOpenAIOutputTokens,
		Store:           false,
	})
	if err != nil {
		return nil, err
	}
	if len(request) > maxOpenAIRequestBytes {
		return nil, fmt.Errorf("OpenAI request exceeds %d bytes", maxOpenAIRequestBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return request, nil
}

func validateOpenAIDailyBriefInputSize(input dailybrief.Input) error {
	if len(input.SourceRecords) > ingestion.MaxRecentSourceRecordsLimit {
		return fmt.Errorf(
			"daily brief input has more than %d source records",
			ingestion.MaxRecentSourceRecordsLimit,
		)
	}
	if len(input.UpcomingEvents) > calendar.MaxUpcomingEventsLimit {
		return fmt.Errorf(
			"daily brief input has more than %d upcoming events",
			calendar.MaxUpcomingEventsLimit,
		)
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
		for _, value := range []string{
			record.ID,
			record.Source,
			record.SourceItemID,
			record.OriginalURL,
			record.Title,
		} {
			if err := add(value); err != nil {
				return err
			}
		}
	}
	for _, event := range input.UpcomingEvents {
		for _, value := range []string{
			event.ID,
			event.Source,
			event.ExternalEventID,
			event.Name,
			string(event.Region),
			string(event.Type),
			event.SourceURL,
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
			ID:           record.ID,
			Source:       record.Source,
			SourceItemID: record.SourceItemID,
			OriginalURL:  record.OriginalURL,
			Title:        record.Title,
			PublishedAt:  formatOpenAITime(record.PublishedAt),
			RetrievedAt:  formatOpenAITime(record.RetrievedAt),
		})
	}
	for _, event := range input.UpcomingEvents {
		providerInput.UpcomingEvents = append(providerInput.UpcomingEvents, openAIDailyBriefUpcomingEvent{
			ID:              event.ID,
			Source:          event.Source,
			ExternalEventID: event.ExternalEventID,
			Name:            event.Name,
			Region:          event.Region,
			Type:            event.Type,
			ScheduledAt:     formatOpenAITime(event.ScheduledAt),
			SourceURL:       event.SourceURL,
			RetrievedAt:     formatOpenAITime(event.RetrievedAt),
		})
	}
	return providerInput
}

func formatOpenAITime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
