package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

func TestRunGeneratesDailyBriefEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	baseEnv := map[string]string{
		"ATLAS_DATABASE_URL":   database.URL,
		"ATLAS_OPENAI_API_KEY": "command-secret",
		"ATLAS_OPENAI_MODEL":   "command-model",
	}
	dependencies := Dependencies{Getenv: func(name string) string { return baseEnv[name] }}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	sourceRepository, err := ingestionpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(source records) error = %v", err)
	}
	eventRepository, err := calendarpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(events) error = %v", err)
	}
	publicationStart := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	for _, record := range []ingestion.SourceRecord{
		commandSourceRecord("older", publicationStart.Add(time.Hour)),
		commandSourceRecord("newer", publicationStart.Add(2*time.Hour)),
	} {
		if _, err := sourceRepository.UpsertSourceRecord(t.Context(), record, "rss-ingestion"); err != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", record.SourceItemID, err)
		}
	}
	eventStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	for _, event := range []calendar.Event{
		commandEvent("first", calendar.RegionUnitedStates, eventStart.Add(time.Hour)),
		commandEvent("second", calendar.RegionUnitedStates, eventStart.Add(2*time.Hour)),
	} {
		if _, err := eventRepository.UpsertEvent(t.Context(), event, "calendar-ingestion"); err != nil {
			t.Fatalf("UpsertEvent(%q) error = %v", event.ExternalEventID, err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer command-secret" {
			t.Errorf("Authorization = %q, want command credential", request.Header.Get("Authorization"))
		}
		var providerRequest openAIDailyBriefRequest
		if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if providerRequest.Model != "command-model" {
			t.Errorf("provider model = %q, want command-model", providerRequest.Model)
		}
		var providerInput openAIDailyBriefInput
		if err := json.Unmarshal([]byte(providerRequest.Input), &providerInput); err != nil {
			t.Errorf("decode provider input: %v", err)
		}
		if len(providerInput.SourceRecords) != 1 || providerInput.SourceRecords[0].SourceItemID != "newer" {
			t.Errorf("provider source records = %#v, want bounded newest record", providerInput.SourceRecords)
		}
		if len(providerInput.UpcomingEvents) != 1 || providerInput.UpcomingEvents[0].ExternalEventID != "first" {
			t.Errorf("provider upcoming events = %#v, want bounded first event", providerInput.UpcomingEvents)
		}
		eventID := "missing-event"
		if len(providerInput.UpcomingEvents) == 1 {
			eventID = providerInput.UpcomingEvents[0].ID
		}
		writeOpenAIResponse(t, response, http.StatusOK, completedOpenAIResponse(`{
			"sections":[{
				"heading":"Next catalyst",
				"content":"The first event is next.",
				"citations":[{"kind":"upcoming_event","id":"`+eventID+`"}]
			}]
		}`))
	}))
	t.Cleanup(server.Close)

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	dependencies.OpenAIHTTPClient = server.Client()
	dependencies.OpenAIEndpoint = server.URL + "/v1/responses"
	err = Run(t.Context(), dailyBriefCommandArguments(), dependencies)
	if err != nil {
		t.Fatalf("Run(daily-brief) error = %v", err)
	}
	var output dailyBriefOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	if output.Region != calendar.RegionUnitedStates || len(output.Sections) != 1 || len(output.Sections[0].Citations) != 1 {
		t.Fatalf("command output = %#v, want one cited United States section", output)
	}
	citation := output.Sections[0].Citations[0]
	if citation.Kind != dailyBriefCitationUpcomingEvent || citation.Source != "example-calendar" ||
		citation.URL != "https://example.com/calendar/first" {
		t.Errorf("command citation = %#v, want canonical first-event citation", citation)
	}

	dependencies.OpenAIHTTPClient = openAIHTTPClientFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"try later"}}`)),
			Header:     make(http.Header),
		}, nil
	})
	dependencies.OpenAIEndpoint = defaultOpenAIResponsesEndpoint
	err = Run(t.Context(), dailyBriefCommandArguments(), dependencies)
	if err == nil || !strings.Contains(err.Error(), "OpenAI Responses API returned status 429") ||
		strings.Contains(err.Error(), "command-secret") {
		t.Fatalf("Run(daily-brief) provider error = %v, want sanitized contextual failure", err)
	}
}

func dailyBriefCommandArguments() []string {
	return []string{
		"daily-brief",
		"--region", "united_states",
		"--publication-from", "2026-07-11T08:00:00Z",
		"--publication-to", "2026-07-11T12:00:00Z",
		"--source-record-limit", "1",
		"--event-from", "2026-07-12T08:00:00Z",
		"--event-to", "2026-07-12T12:00:00Z",
		"--upcoming-event-limit", "1",
	}
}

type openAIHTTPClientFunc func(*http.Request) (*http.Response, error)

func (client openAIHTTPClientFunc) Do(request *http.Request) (*http.Response, error) {
	return client(request)
}
