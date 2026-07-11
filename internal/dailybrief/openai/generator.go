package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/dailybrief"
)

const (
	defaultOpenAIResponsesEndpoint = "https://api.openai.com/v1/responses"
	defaultOpenAIRequestBudget     = 30 * time.Second
	maxOpenAIModelBytes            = 256
	maxOpenAIDailyBriefInputBytes  = 256 << 10
	maxOpenAIRequestBytes          = 4 << 20
	maxOpenAIResponseBytes         = 1 << 20
	maxOpenAIOutputTokens          = 4096
)

// HTTPClient executes OpenAI Responses API requests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Config configures a Responses API daily-brief generator.
type Config struct {
	APIKey        string
	Model         string
	Client        HTTPClient
	Endpoint      string
	RequestBudget time.Duration
}

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
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return nil, errors.New("OpenAI API key is required")
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		return nil, errors.New("OpenAI model is required")
	}
	if len(model) > maxOpenAIModelBytes {
		return nil, fmt.Errorf("OpenAI model must not exceed %d bytes", maxOpenAIModelBytes)
	}
	if config.RequestBudget < 0 {
		return nil, errors.New("OpenAI request budget must not be negative")
	}

	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = defaultOpenAIResponsesEndpoint
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || (parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") ||
		parsedEndpoint.Hostname() == "" || parsedEndpoint.User != nil || parsedEndpoint.Fragment != "" {
		return nil, errors.New("OpenAI endpoint must be an absolute HTTP(S) URL without credentials or a fragment")
	}
	if parsedEndpoint.Scheme != "https" && !isLoopbackHost(parsedEndpoint.Hostname()) {
		return nil, errors.New("OpenAI endpoint must use HTTPS unless it targets a loopback host")
	}

	requestBudget := config.RequestBudget
	if requestBudget == 0 {
		requestBudget = defaultOpenAIRequestBudget
	}
	client := config.Client
	if client == nil {
		client = &http.Client{
			Timeout: requestBudget,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	return &Generator{
		apiKey:        apiKey,
		model:         model,
		client:        client,
		endpoint:      endpoint,
		requestBudget: requestBudget,
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

func isLoopbackHost(host string) bool {
	if strings.TrimSuffix(strings.ToLower(host), ".") == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func openAIProviderError(statusCode int, body []byte) error {
	var response openAIErrorResponse
	if err := json.Unmarshal(body, &response); err != nil || response.Error == nil {
		return fmt.Errorf("OpenAI Responses API returned status %d", statusCode)
	}

	parts := make([]string, 0, 3)
	for _, value := range []string{response.Error.Type, response.Error.Code, response.Error.Message} {
		if value = sanitizeOpenAIErrorValue(value); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return fmt.Errorf("OpenAI Responses API returned status %d", statusCode)
	}
	return fmt.Errorf("OpenAI Responses API returned status %d: %s", statusCode, strings.Join(parts, ": "))
}

func sanitizeOpenAIErrorValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maxLength = 256
	if len(value) > maxLength {
		return value[:maxLength] + "..."
	}
	return value
}
