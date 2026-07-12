package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Yanis897349/atlas/internal/dailybrief"
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
