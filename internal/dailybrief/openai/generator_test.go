package openai

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

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestOpenAIDailyBriefGeneratorRequestsStructuredResponse(t *testing.T) {
	input := generationInput()
	wantDraft := dailybrief.Draft{Sections: []dailybrief.SectionDraft{
		{
			Heading: "Growth is slowing",
			Content: "Recent reporting points to softer activity.",
			Citations: []dailybrief.CitationReference{
				{Kind: dailybrief.CitationSourceRecord, ID: "record-news"},
				{Kind: dailybrief.CitationUpcomingEvent, ID: "event-gdp"},
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
	want := dailybrief.Generation{Provider: "openai", Model: "test-model", Draft: wantDraft}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Generate() = %#v, want %#v", got, want)
	}
}

func TestNewOpenAIDailyBriefGeneratorValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing API key", config: Config{Model: "model"}, want: "API key is required"},
		{name: "missing model", config: Config{APIKey: "key"}, want: "model is required"},
		{name: "oversized model", config: Config{
			APIKey: "key", Model: strings.Repeat("m", maxOpenAIModelBytes+1),
		}, want: "model must not exceed"},
		{name: "negative request budget", config: Config{
			APIKey: "key", Model: "model", RequestBudget: -time.Second,
		}, want: "request budget must not be negative"},
		{name: "relative endpoint", config: Config{
			APIKey: "key", Model: "model", Endpoint: "/v1/responses",
		}, want: "absolute HTTP(S) URL"},
		{name: "unsupported endpoint scheme", config: Config{
			APIKey: "key", Model: "model", Endpoint: "file:///tmp/responses",
		}, want: "absolute HTTP(S) URL"},
		{name: "remote plaintext endpoint", config: Config{
			APIKey: "key", Model: "model", Endpoint: "http://api.example.com/v1/responses",
		}, want: "must use HTTPS unless it targets a loopback host"},
		{name: "endpoint credentials", config: Config{
			APIKey: "key", Model: "model", Endpoint: "https://user@example.com/v1/responses",
		}, want: "without credentials or a fragment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generator, err := NewGenerator(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("newOpenAIDailyBriefGenerator() = (%#v, %v), want error containing %q", generator, err, test.want)
			}
		})
	}

	generator, err := NewGenerator(Config{
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

func TestOpenAIDailyBriefGeneratorRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(strings.Repeat("x", maxOpenAIResponseBytes+1)))
	}))
	defer server.Close()

	_, err := newOpenAITestGenerator(t, server).Generate(t.Context(), generationInput())
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

	_, err := newOpenAITestGenerator(t, server).Generate(t.Context(), generationInput())
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
		_, err := generator.Generate(ctx, generationInput())
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

func newOpenAITestGenerator(t *testing.T, server *httptest.Server) *Generator {
	t.Helper()
	generator, err := NewGenerator(Config{
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

func generationInput() dailybrief.Input {
	return dailybrief.Input{
		Region:                 calendar.RegionUnitedStates,
		PublicationWindowStart: time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC),
		PublicationWindowEnd:   time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC),
		EventWindowStart:       time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC),
		EventWindowEnd:         time.Date(2026, time.July, 12, 0, 0, 0, 0, time.UTC),
		SourceRecords: []ingestion.StoredSourceRecord{{
			ID: "record-news",
			SourceRecord: ingestion.SourceRecord{
				Source:       "example-news",
				SourceItemID: "news-1",
				OriginalURL:  "https://example.com/news/1",
				Title:        "Growth report",
				PublishedAt:  time.Date(2026, time.July, 10, 8, 0, 0, 0, time.UTC),
				RetrievedAt:  time.Date(2026, time.July, 10, 9, 0, 0, 0, time.UTC),
			},
		}},
		UpcomingEvents: []calendar.StoredEvent{{
			ID: "event-gdp",
			Event: calendar.Event{
				Source:          "official-calendar",
				ExternalEventID: "gdp-1",
				Name:            "GDP release",
				Region:          calendar.RegionUnitedStates,
				Type:            calendar.EventTypeGDP,
				ScheduledAt:     time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC),
				SourceURL:       "https://example.com/calendar/gdp-1",
				RetrievedAt:     time.Date(2026, time.July, 10, 9, 0, 0, 0, time.UTC),
			},
		}},
	}
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
