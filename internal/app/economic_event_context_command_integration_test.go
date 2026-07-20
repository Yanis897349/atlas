package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/jackc/pgx/v5"
)

func TestRunAssemblesEconomicEventContextEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	env := map[string]string{
		"ATLAS_DATABASE_URL":           database.URL,
		"ATLAS_OPENAI_API_KEY":         "context-secret",
		"ATLAS_OPENAI_EMBEDDING_MODEL": "embedding-model",
	}
	dependencies := Dependencies{Getenv: func(name string) string { return env[name] }}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	eventRepository, err := calendarpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(events) error = %v", err)
	}
	windowStart := time.Date(2026, time.July, 12, 8, 0, 0, 123000000, time.UTC)
	windowEnd := windowStart.Add(4 * time.Hour)
	event, err := eventRepository.UpsertEvent(t.Context(), calendar.Event{
		Source:          "official-calendar",
		ExternalEventID: "cpi-2026-07",
		Name:            "  Consumer Price Index  ",
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeInflation,
		ScheduledAt:     windowEnd.Add(24 * time.Hour),
		SourceURL:       "https://example.com/calendar/cpi-2026-07",
		RetrievedAt:     windowStart.Add(-time.Hour),
	}, "calendar-ingestion")
	if err != nil {
		t.Fatalf("UpsertEvent() error = %v", err)
	}
	emptyEvent, err := eventRepository.UpsertEvent(t.Context(), calendar.Event{
		Source:          "official-calendar",
		ExternalEventID: "cpi-empty-2026-07",
		Name:            event.Name,
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeInflation,
		ScheduledAt:     windowEnd.Add(48 * time.Hour),
		SourceURL:       "https://example.com/calendar/cpi-empty-2026-07",
		RetrievedAt:     windowStart.Add(-time.Hour),
	}, "calendar-ingestion")
	if err != nil {
		t.Fatalf("UpsertEvent(empty) error = %v", err)
	}
	observationFixture := storeEconomicEventContextObservations(t, database.Pool, event.ID, windowEnd)
	records := storeEconomicEventContextSourceRecords(t, database.Pool, windowStart, windowEnd)

	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		providerCalls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("request = %s %s, want POST /v1/embeddings", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer context-secret" {
			t.Errorf("Authorization = %q, want context credential", request.Header.Get("Authorization"))
		}
		var providerRequest struct {
			Model          string   `json:"model"`
			Input          []string `json:"input"`
			EncodingFormat string   `json:"encoding_format"`
		}
		if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if !reflect.DeepEqual(providerRequest.Input, []string{event.Name}) ||
			providerRequest.EncodingFormat != "float" {
			t.Errorf("provider request = %#v, want exact persisted event name", providerRequest)
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(map[string]any{
			"object": "list",
			"model":  providerRequest.Model,
			"data": []map[string]any{{
				"object": "embedding", "index": 0, "embedding": []float32{1, 0},
			}},
		}); err != nil {
			t.Errorf("encode provider response: %v", err)
		}
	}))
	t.Cleanup(provider.Close)
	dependencies.OpenAIHTTPClient = provider.Client()
	dependencies.OpenAIEmbeddingsEndpoint = provider.URL + "/v1/embeddings"
	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout

	arguments := []string{
		"economic-event-context",
		"--event-id", event.ID,
		"--from", windowStart.Format(time.RFC3339Nano),
		"--to", windowEnd.Format(time.RFC3339Nano),
		"--limit", "4",
		"--observation-limit", "5",
		"--observation-revision-limit", "2",
	}
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(economic-event-context) error = %v", err)
	}
	var output economicEventContextIntegrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	assertEconomicEventContextIntegrationOutput(t, output, economicEventContextIntegrationWant{
		rawOutput:        stdout.Bytes(),
		event:            event,
		windowStart:      windowStart,
		windowEnd:        windowEnd,
		observations:     observationFixture.stored,
		latestInitial:    observationFixture.latestInitial,
		latestRevision:   observationFixture.latestRevision,
		officialInitial:  observationFixture.officialInitial,
		officialCitation: observationFixture.officialCitation,
		officialLatest:   observationFixture.officialLatest,
		consensus:        observationFixture.consensus,
		previous:         observationFixture.previous,
		actual:           observationFixture.actual,
		revisedActual:    observationFixture.revisedActual,
		sourceRecords:    records,
	})

	stdout.Reset()
	env["ATLAS_OPENAI_EMBEDDING_MODEL"] = "unindexed-model"
	arguments[2] = emptyEvent.ID
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(economic-event-context empty) error = %v", err)
	}
	output = economicEventContextIntegrationOutput{}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode empty command output: %v", err)
	}
	if output.Observations == nil || len(output.Observations) != 0 ||
		output.SourceRecords == nil || len(output.SourceRecords) != 0 || providerCalls.Load() != 2 {
		t.Errorf("empty arrays = (%#v, %#v) with %d calls, want [] after second embedding", output.Observations, output.SourceRecords, providerCalls.Load())
	}

	stdout.Reset()
	arguments[2] = "00000000-0000-0000-0000-000000000999"
	err = Run(t.Context(), arguments, dependencies)
	if !errors.Is(err, pgx.ErrNoRows) || stdout.Len() != 0 || providerCalls.Load() != 2 {
		t.Fatalf("missing event = (%v, %q, %d calls), want pgx.ErrNoRows without output or provider call", err, stdout.String(), providerCalls.Load())
	}
}
