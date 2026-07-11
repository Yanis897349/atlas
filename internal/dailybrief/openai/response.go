package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/dailybrief"
)

type openAIResponsesResponse struct {
	Status            string                     `json:"status"`
	Error             *openAIError               `json:"error"`
	IncompleteDetails *openAIIncompleteDetails   `json:"incomplete_details"`
	Output            []openAIResponseOutputItem `json:"output"`
}

type openAIErrorResponse struct {
	Error *openAIError `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

type openAIIncompleteDetails struct {
	Reason string `json:"reason"`
}

type openAIResponseOutputItem struct {
	Type    string                      `json:"type"`
	Role    string                      `json:"role"`
	Content []openAIResponseContentItem `json:"content"`
}

type openAIResponseContentItem struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

type openAIDraftOutput struct {
	Sections []openAISectionDraftOutput `json:"sections"`
}

type openAISectionDraftOutput struct {
	Heading   string                      `json:"heading"`
	Content   string                      `json:"content"`
	Citations []openAICitationDraftOutput `json:"citations"`
}

type openAICitationDraftOutput struct {
	Kind dailybrief.CitationKind `json:"kind"`
	ID   string                  `json:"id"`
}

func decodeOpenAIDailyBriefResponse(body []byte) (dailybrief.Draft, error) {
	var response openAIResponsesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return dailybrief.Draft{}, fmt.Errorf("decode response envelope: %w", err)
	}
	if response.Status != "completed" {
		return dailybrief.Draft{}, openAIIncompleteResponseError(response)
	}
	if response.Error != nil {
		return dailybrief.Draft{}, errors.New("completed response contains a provider error")
	}

	var outputText string
	messageCount := 0
	for _, output := range response.Output {
		switch output.Type {
		case "reasoning":
			continue
		case "message":
			messageCount++
		default:
			return dailybrief.Draft{}, fmt.Errorf("unexpected output item type %q", output.Type)
		}
		if output.Role != "assistant" {
			return dailybrief.Draft{}, fmt.Errorf("unexpected message role %q", output.Role)
		}
		if len(output.Content) != 1 {
			return dailybrief.Draft{}, fmt.Errorf("assistant message has %d content items, want 1", len(output.Content))
		}
		content := output.Content[0]
		switch content.Type {
		case "refusal":
			return dailybrief.Draft{}, errors.New("OpenAI refused to generate the daily brief")
		case "output_text":
			outputText = content.Text
		default:
			return dailybrief.Draft{}, fmt.Errorf("unexpected message content type %q", content.Type)
		}
	}
	if messageCount != 1 {
		return dailybrief.Draft{}, fmt.Errorf("response has %d assistant messages, want 1", messageCount)
	}

	var providerDraft openAIDraftOutput
	decoder := json.NewDecoder(bytes.NewBufferString(outputText))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&providerDraft); err != nil {
		return dailybrief.Draft{}, fmt.Errorf("decode structured daily brief: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return dailybrief.Draft{}, fmt.Errorf("decode structured daily brief: %w", err)
	}

	draft := dailybrief.Draft{Sections: make([]dailybrief.SectionDraft, 0, len(providerDraft.Sections))}
	for _, providerSection := range providerDraft.Sections {
		section := dailybrief.SectionDraft{
			Heading:   providerSection.Heading,
			Content:   providerSection.Content,
			Citations: make([]dailybrief.CitationReference, 0, len(providerSection.Citations)),
		}
		for _, citation := range providerSection.Citations {
			section.Citations = append(section.Citations, dailybrief.CitationReference{
				Kind: citation.Kind,
				ID:   citation.ID,
			})
		}
		draft.Sections = append(draft.Sections, section)
	}
	return draft, nil
}

func openAIIncompleteResponseError(response openAIResponsesResponse) error {
	if response.Status == "incomplete" && response.IncompleteDetails != nil {
		reason := sanitizeOpenAIErrorValue(response.IncompleteDetails.Reason)
		if reason != "" {
			return fmt.Errorf("OpenAI response is incomplete: %s", reason)
		}
	}
	if response.Status == "failed" && response.Error != nil {
		return errors.New("OpenAI response failed")
	}
	return fmt.Errorf("OpenAI response has unexpected status %q", response.Status)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}
