package openai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Yanis897349/atlas/internal/dailybrief"
	openaiapi "github.com/Yanis897349/atlas/internal/openai"
)

const (
	defaultOpenAIResponsesEndpoint = "https://api.openai.com/v1/responses"
	maxOpenAIDailyBriefInputBytes  = 256 << 10
	maxOpenAIRequestBytes          = 4 << 20
	maxOpenAIResponseBytes         = 1 << 20
	maxOpenAIOutputTokens          = 4096
)

// HTTPClient executes OpenAI Responses API requests.
type HTTPClient = openaiapi.HTTPClient

// Config configures a Responses API daily-brief generator.
type Config = openaiapi.Config

// Generator creates daily-brief drafts through the OpenAI Responses API.
type Generator struct {
	apiKey        string
	model         string
	client        HTTPClient
	endpoint      string
	requestBudget time.Duration
}

var _ dailybrief.Generator = (*Generator)(nil)

// NewGenerator returns a validated OpenAI daily-brief generator.
func NewGenerator(config Config) (*Generator, error) {
	resolved, err := openaiapi.ResolveConfig(config, defaultOpenAIResponsesEndpoint)
	if err != nil {
		return nil, err
	}

	return &Generator{
		apiKey:        resolved.APIKey,
		model:         resolved.Model,
		client:        resolved.Client,
		endpoint:      resolved.Endpoint,
		requestBudget: resolved.RequestBudget,
	}, nil
}

func (generator *Generator) Generate(
	ctx context.Context,
	input dailybrief.Input,
) (dailybrief.Generation, error) {
	requestContext, cancel := context.WithTimeout(ctx, generator.requestBudget)
	defer cancel()

	requestBody, err := newOpenAIDailyBriefRequest(requestContext, generator.model, input)
	if err != nil {
		return dailybrief.Generation{}, fmt.Errorf("encode OpenAI daily brief request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		generator.endpoint,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return dailybrief.Generation{}, fmt.Errorf("create OpenAI Responses API request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+generator.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := generator.client.Do(request)
	if err != nil {
		return dailybrief.Generation{}, fmt.Errorf("send OpenAI Responses API request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxOpenAIResponseBytes+1))
	if err != nil {
		return dailybrief.Generation{}, fmt.Errorf("read OpenAI Responses API response: %w", err)
	}
	if len(responseBody) > maxOpenAIResponseBytes {
		return dailybrief.Generation{}, fmt.Errorf(
			"read OpenAI Responses API response: body exceeds %d bytes",
			maxOpenAIResponseBytes,
		)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return dailybrief.Generation{}, openAIProviderError(response.StatusCode, responseBody)
	}

	draft, err := decodeOpenAIDailyBriefResponse(responseBody)
	if err != nil {
		return dailybrief.Generation{}, fmt.Errorf("decode OpenAI Responses API response: %w", err)
	}
	return dailybrief.Generation{Provider: "openai", Model: generator.model, Draft: draft}, nil
}

func openAIProviderError(statusCode int, body []byte) error {
	return openaiapi.ProviderError("OpenAI Responses API", statusCode, body)
}
