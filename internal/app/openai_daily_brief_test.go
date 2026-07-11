package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOpenAIDailyBriefGeneratorRequestsStructuredResponse(t *testing.T) {
	input := dailyBriefGenerationInput()
	wantDraft := dailyBriefDraft{sections: []dailyBriefSectionDraft{
		{
			heading: "Growth is slowing",
			content: "Recent reporting points to softer activity.",
			citations: []dailyBriefCitationReference{
				{kind: dailyBriefCitationSourceRecord, id: "record-news"},
				{kind: dailyBriefCitationUpcomingEvent, id: "event-gdp"},
			},
		},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			t.Errorf("request = %s %s, want POST /v1/responses", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("Authorization = %q, want bearer test credential", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var providerRequest openAIDailyBriefRequest
		if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if providerRequest.Model != "test-model" || providerRequest.Instructions != openAIDailyBriefInstructions {
			t.Errorf("request model/instructions = (%q, %q)", providerRequest.Model, providerRequest.Instructions)
		}
		if providerRequest.MaxOutputTokens != maxOpenAIOutputTokens || providerRequest.Store {
			t.Errorf("request bounds = (tokens %d, store %t)", providerRequest.MaxOutputTokens, providerRequest.Store)
		}
		format := providerRequest.Text.Format
		if format.Type != "json_schema" || format.Name != "daily_brief" || !format.Strict ||
			!jsonEqual(format.Schema, openAIDailyBriefJSONSchema) {
			t.Errorf("response format = %#v, want strict daily brief schema", format)
		}

		var providerInput openAIDailyBriefInput
		if err := json.Unmarshal([]byte(providerRequest.Input), &providerInput); err != nil {
			t.Errorf("decode provider input: %v", err)
		}
		wantInput := newOpenAIDailyBriefInput(input)
		if !reflect.DeepEqual(providerInput, wantInput) {
			t.Errorf("provider input = %#v, want %#v", providerInput, wantInput)
		}

		writeOpenAIResponse(t, response, http.StatusOK, completedOpenAIResponse(`{
			"sections": [{
				"heading": "Growth is slowing",
				"content": "Recent reporting points to softer activity.",
				"citations": [
					{"kind": "source_record", "id": "record-news"},
					{"kind": "upcoming_event", "id": "event-gdp"}
				]
			}]
		}`))
	}))
	defer server.Close()

	generator := newOpenAITestGenerator(t, server)
	got, err := generator.Generate(t.Context(), input)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !reflect.DeepEqual(got, wantDraft) {
		t.Errorf("Generate() = %#v, want %#v", got, wantDraft)
	}
}

func TestNewOpenAIDailyBriefGeneratorValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config openAIDailyBriefGeneratorConfig
		want   string
	}{
		{name: "missing API key", config: openAIDailyBriefGeneratorConfig{Model: "model"}, want: "API key is required"},
		{name: "missing model", config: openAIDailyBriefGeneratorConfig{APIKey: "key"}, want: "model is required"},
		{name: "oversized model", config: openAIDailyBriefGeneratorConfig{
			APIKey: "key", Model: strings.Repeat("m", maxOpenAIModelBytes+1),
		}, want: "model must not exceed"},
		{name: "negative request budget", config: openAIDailyBriefGeneratorConfig{
			APIKey: "key", Model: "model", RequestBudget: -time.Second,
		}, want: "request budget must not be negative"},
		{name: "relative endpoint", config: openAIDailyBriefGeneratorConfig{
			APIKey: "key", Model: "model", Endpoint: "/v1/responses",
		}, want: "absolute HTTP(S) URL"},
		{name: "unsupported endpoint scheme", config: openAIDailyBriefGeneratorConfig{
			APIKey: "key", Model: "model", Endpoint: "file:///tmp/responses",
		}, want: "absolute HTTP(S) URL"},
		{name: "remote plaintext endpoint", config: openAIDailyBriefGeneratorConfig{
			APIKey: "key", Model: "model", Endpoint: "http://api.example.com/v1/responses",
		}, want: "must use HTTPS unless it targets a loopback host"},
		{name: "endpoint credentials", config: openAIDailyBriefGeneratorConfig{
			APIKey: "key", Model: "model", Endpoint: "https://user@example.com/v1/responses",
		}, want: "without credentials or a fragment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generator, err := newOpenAIDailyBriefGenerator(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("newOpenAIDailyBriefGenerator() = (%#v, %v), want error containing %q", generator, err, test.want)
			}
		})
	}

	generator, err := newOpenAIDailyBriefGenerator(openAIDailyBriefGeneratorConfig{
		APIKey: " key ", Model: " model ",
	})
	if err != nil {
		t.Fatalf("newOpenAIDailyBriefGenerator() error = %v", err)
	}
	if generator.apiKey != "key" || generator.model != "model" ||
		generator.endpoint != defaultOpenAIResponsesEndpoint ||
		generator.requestBudget != defaultOpenAIRequestBudget || generator.client == nil {
		t.Errorf("generator defaults = %#v", generator)
	}
	client, ok := generator.client.(*http.Client)
	if !ok || client.CheckRedirect == nil ||
		!errors.Is(client.CheckRedirect(&http.Request{}, nil), http.ErrUseLastResponse) {
		t.Errorf("default HTTP client must reject redirects")
	}
}

func TestOpenAIDailyBriefGeneratorBoundsRequestConstruction(t *testing.T) {
	tests := []struct {
		name     string
		ctx      func() context.Context
		input    func() dailyBriefInput
		contains string
	}{
		{
			name: "cancelled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			input:    dailyBriefGenerationInput,
			contains: "context canceled",
		},
		{
			name: "oversized input",
			ctx:  t.Context,
			input: func() dailyBriefInput {
				input := dailyBriefGenerationInput()
				input.sourceRecords[0].Title = strings.Repeat("x", maxOpenAIDailyBriefInputBytes+1)
				return input
			},
			contains: "daily brief input is too large",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &openAIHTTPClientStub{}
			generator, err := newOpenAIDailyBriefGenerator(openAIDailyBriefGeneratorConfig{
				APIKey: "key", Model: "model", Client: client,
			})
			if err != nil {
				t.Fatalf("newOpenAIDailyBriefGenerator() error = %v", err)
			}

			_, err = generator.Generate(test.ctx(), test.input())
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Generate() error = %v, want error containing %q", err, test.contains)
			}
			if client.calls != 0 {
				t.Errorf("HTTP calls = %d, want 0", client.calls)
			}
		})
	}
}

func TestOpenAIDailyBriefGeneratorRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		contains string
	}{
		{name: "malformed envelope", body: `{`, contains: "decode response envelope"},
		{name: "incomplete", body: `{
			"status":"incomplete",
			"incomplete_details":{"reason":"max_output_tokens"}
		}`, contains: "response is incomplete: max_output_tokens"},
		{name: "failed", body: `{
			"status":"failed",
			"error":{"message":"internal details","type":"server_error","code":"failed"}
		}`, contains: "response failed"},
		{name: "completed with error", body: `{
			"status":"completed",
			"error":{"message":"internal details","type":"server_error","code":"failed"}
		}`, contains: "completed response contains a provider error"},
		{name: "missing message", body: `{"status":"completed","output":[]}`, contains: "0 assistant messages"},
		{name: "duplicate message", body: `{
			"status":"completed",
			"output":[
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{}"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{}"}]}
			]
		}`, contains: "2 assistant messages"},
		{name: "unexpected output", body: `{
			"status":"completed",
			"output":[{"type":"function_call"}]
		}`, contains: `unexpected output item type "function_call"`},
		{name: "missing content", body: `{
			"status":"completed",
			"output":[{"type":"message","role":"assistant","content":[]}]
		}`, contains: "0 content items, want 1"},
		{name: "unexpected content", body: `{
			"status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"image"}]}]
		}`, contains: `unexpected message content type "image"`},
		{name: "refusal", body: `{
			"status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"no"}]}]
		}`, contains: "refused to generate"},
		{name: "malformed structured output", body: completedOpenAIResponse(`{`), contains: "decode structured daily brief"},
		{name: "unknown structured field", body: completedOpenAIResponse(`{
			"sections": [], "unexpected": true
		}`), contains: `unknown field "unexpected"`},
		{name: "trailing structured JSON", body: completedOpenAIResponse(`{
			"sections": []
		} {}`), contains: "unexpected trailing JSON value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				writeOpenAIResponse(t, response, http.StatusOK, test.body)
			}))
			defer server.Close()

			got, err := newOpenAITestGenerator(t, server).Generate(t.Context(), dailyBriefGenerationInput())
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Generate() error = %v, want error containing %q", err, test.contains)
			}
			if !reflect.DeepEqual(got, dailyBriefDraft{}) {
				t.Errorf("Generate() = %#v, want zero value", got)
			}
		})
	}
}

func TestOpenAIDailyBriefGeneratorRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(strings.Repeat("x", maxOpenAIResponseBytes+1)))
	}))
	defer server.Close()

	_, err := newOpenAITestGenerator(t, server).Generate(t.Context(), dailyBriefGenerationInput())
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("body exceeds %d bytes", maxOpenAIResponseBytes)) {
		t.Fatalf("Generate() error = %v, want oversized response error", err)
	}
}

func TestOpenAIDailyBriefGeneratorSanitizesProviderErrors(t *testing.T) {
	message := "quota\nexceeded " + strings.Repeat("private ", 100)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeOpenAIResponse(t, response, http.StatusTooManyRequests, fmt.Sprintf(`{
			"error":{"message":%q,"type":"rate_limit_error","code":"rate_limit_exceeded"}
		}`, message))
	}))
	defer server.Close()

	_, err := newOpenAITestGenerator(t, server).Generate(t.Context(), dailyBriefGenerationInput())
	if err == nil || !strings.Contains(err.Error(), "status 429: rate_limit_error: rate_limit_exceeded: quota exceeded") {
		t.Fatalf("Generate() error = %v, want structured provider error", err)
	}
	if strings.Contains(err.Error(), "\n") || len(err.Error()) > 400 || strings.Contains(err.Error(), "test-secret") {
		t.Errorf("Generate() exposed unsanitized provider data: %q", err)
	}
}

func TestOpenAIDailyBriefGeneratorPreservesCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()
	generator := newOpenAITestGenerator(t, server)

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := generator.Generate(ctx, dailyBriefGenerationInput())
		result <- err
	}()
	<-started
	cancel()

	select {
	case err := <-result:
		close(release)
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "send OpenAI Responses API request") {
			t.Fatalf("Generate() error = %v, want contextual cancellation", err)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("Generate() did not return after cancellation")
	}
}

func newOpenAITestGenerator(t *testing.T, server *httptest.Server) *openAIDailyBriefGenerator {
	t.Helper()
	generator, err := newOpenAIDailyBriefGenerator(openAIDailyBriefGeneratorConfig{
		APIKey:        "test-secret",
		Model:         "test-model",
		Client:        server.Client(),
		Endpoint:      server.URL + "/v1/responses",
		RequestBudget: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("newOpenAIDailyBriefGenerator() error = %v", err)
	}
	return generator
}

func completedOpenAIResponse(outputText string) string {
	encodedText, err := json.Marshal(outputText)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{
		"status":"completed",
		"output":[
			{"type":"reasoning"},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":%s}]}
		]
	}`, encodedText)
}

func writeOpenAIResponse(t *testing.T, response http.ResponseWriter, status int, body string) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if _, err := response.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}

type openAIHTTPClientStub struct {
	calls int
}

func (client *openAIHTTPClientStub) Do(*http.Request) (*http.Response, error) {
	client.calls++
	return nil, errors.New("unexpected HTTP request")
}
