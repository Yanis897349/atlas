package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
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
	observationRepository, err := intelligencepostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(observations) error = %v", err)
	}
	consensus, previous, actual := "3.1%", "3.0%", "3.3%"
	observationFixtures := []intelligence.Observation{
		{
			EconomicEventID:     event.ID,
			Source:              "oldest-statistics",
			SourceObservationID: "cpi-oldest-2026-07",
			SourceURL:           "https://example.com/releases/cpi-oldest-2026-07",
			ObservedAt:          windowEnd.Add(time.Hour),
			Consensus:           &consensus,
		},
		{
			EconomicEventID:     event.ID,
			Source:              "official-statistics",
			SourceObservationID: "cpi-2026-07",
			SourceURL:           "https://example.com/releases/cpi-2026-07",
			ObservedAt:          windowEnd.Add(2 * time.Hour),
			Consensus:           &consensus,
			Previous:            &previous,
			Actual:              &actual,
		},
		{
			EconomicEventID:     event.ID,
			Source:              "latest-statistics",
			SourceObservationID: "cpi-latest-2026-07",
			SourceURL:           "https://example.com/releases/cpi-latest-2026-07",
			ObservedAt:          windowEnd.Add(3 * time.Hour),
			Previous:            &previous,
		},
	}
	storedObservations := make(map[string]intelligence.StoredObservation, len(observationFixtures))
	for _, fixture := range observationFixtures {
		stored, persistErr := observationRepository.StoreObservation(
			t.Context(), fixture, "observation-ingestion",
		)
		if persistErr != nil {
			t.Fatalf("StoreObservation(%q) error = %v", fixture.SourceObservationID, persistErr)
		}
		storedObservations[fixture.SourceObservationID] = stored
	}

	sourceRepository, err := ingestionpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(source records) error = %v", err)
	}
	type sourceFixture struct {
		itemID      string
		publishedAt time.Time
		vector      []float32
		model       string
	}
	fixtures := []sourceFixture{
		{itemID: "before", publishedAt: windowStart.Add(-time.Microsecond), vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "start", publishedAt: windowStart, vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "middle-a", publishedAt: windowStart.Add(time.Hour), vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "middle-b", publishedAt: windowStart.Add(2 * time.Hour), vector: []float32{1, 1}, model: "embedding-model"},
		{itemID: "end", publishedAt: windowEnd, vector: []float32{0, 1}, model: "embedding-model"},
		{itemID: "after", publishedAt: windowEnd.Add(time.Microsecond), vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "other-model", publishedAt: windowStart.Add(3 * time.Hour), vector: []float32{1, 0}, model: "other-model"},
		{itemID: "other-dimension", publishedAt: windowStart.Add(3 * time.Hour), vector: []float32{1, 0, 0}, model: "embedding-model"},
	}
	records := make(map[string]ingestion.StoredSourceRecord, len(fixtures))
	embeddings := make([]search.SourceRecordEmbedding, 0, len(fixtures))
	for _, fixture := range fixtures {
		record, persistErr := sourceRepository.UpsertSourceRecord(t.Context(), ingestion.SourceRecord{
			Source:       "test-publisher",
			SourceItemID: fixture.itemID,
			OriginalURL:  "https://example.com/news/" + fixture.itemID,
			Title:        "Story " + fixture.itemID,
			PublishedAt:  fixture.publishedAt,
			RetrievedAt:  fixture.publishedAt.Add(time.Minute),
		}, "rss-ingestion")
		if persistErr != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", fixture.itemID, persistErr)
		}
		records[fixture.itemID] = record
		embeddings = append(embeddings, search.SourceRecordEmbedding{
			SourceRecordID: record.ID,
			Provider:       "openai",
			Model:          fixture.model,
			Vector:         fixture.vector,
		})
	}
	embeddingRepository, err := searchpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(embeddings) error = %v", err)
	}
	for index, batch := range [][]search.SourceRecordEmbedding{embeddings[:len(embeddings)-1], embeddings[len(embeddings)-1:]} {
		if err := embeddingRepository.PersistSourceRecordEmbeddings(
			t.Context(), batch, "search-indexer",
		); err != nil {
			t.Fatalf("PersistSourceRecordEmbeddings(batch %d) error = %v", index, err)
		}
	}

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
		"--observation-limit", "2",
	}
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(economic-event-context) error = %v", err)
	}
	var output economicEventContextIntegrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	var outputFields map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &outputFields); err != nil {
		t.Fatalf("decode command output fields: %v", err)
	}
	if _, exists := outputFields["observations"]; !exists || len(outputFields) != 5 {
		t.Errorf("command output fields = %v, want schema with observations", outputFields)
	}
	if output.Event.ID != event.ID || output.Event.Source != event.Source ||
		output.Event.ExternalEventID != event.ExternalEventID || output.Event.Name != event.Name ||
		output.Event.Region != event.Region || output.Event.EventType != event.Type ||
		output.Event.SourceURL != event.SourceURL || output.Event.ScheduledAt == "" ||
		output.Event.RetrievedAt == "" || output.Event.CreatedAt == "" || output.Event.UpdatedAt == "" ||
		output.Event.CreatedBy != "calendar-ingestion" || output.Event.UpdatedBy != "calendar-ingestion" {
		t.Errorf("event output = %#v, want complete canonical event", output.Event)
	}
	if output.PublicationWindowStart != windowStart.Format(time.RFC3339Nano) ||
		output.PublicationWindowEnd != windowEnd.Format(time.RFC3339Nano) {
		t.Errorf("publication window = (%q, %q), want normalized inclusive bounds", output.PublicationWindowStart, output.PublicationWindowEnd)
	}
	wantObservationIDs := []string{
		storedObservations["cpi-latest-2026-07"].ID,
		storedObservations["cpi-2026-07"].ID,
	}
	if len(output.Observations) != len(wantObservationIDs) {
		t.Fatalf("observations = %#v, want two bounded observations", output.Observations)
	}
	for index, wantID := range wantObservationIDs {
		got := output.Observations[index]
		if got.ID != wantID || got.EconomicEventID != event.ID || got.Source == "" ||
			got.SourceObservationID == "" || got.SourceURL == "" || got.ObservedAt == "" ||
			got.CreatedAt == "" || got.UpdatedAt == "" || got.CreatedBy != "observation-ingestion" ||
			got.UpdatedBy != "observation-ingestion" {
			t.Errorf("observations[%d] = %#v, want complete canonical observation %q", index, got, wantID)
		}
	}
	if output.Observations[0].Consensus != nil || output.Observations[0].Actual != nil ||
		output.Observations[0].Previous == nil || *output.Observations[0].Previous != previous ||
		output.Observations[1].Consensus == nil || *output.Observations[1].Consensus != consensus ||
		output.Observations[1].Previous == nil || *output.Observations[1].Previous != previous ||
		output.Observations[1].Actual == nil || *output.Observations[1].Actual != actual {
		t.Errorf("observation values = %#v, want exact nullable values", output.Observations)
	}
	exact := []ingestion.StoredSourceRecord{records["start"], records["middle-a"]}
	sort.Slice(exact, func(left, right int) bool { return exact[left].ID < exact[right].ID })
	wantIDs := []string{exact[0].ID, exact[1].ID, records["middle-b"].ID, records["end"].ID}
	if len(output.SourceRecords) != len(wantIDs) {
		t.Fatalf("source records = %#v, want four inclusive matching records", output.SourceRecords)
	}
	for index, wantID := range wantIDs {
		got := output.SourceRecords[index]
		if got.ID != wantID || got.Source != "test-publisher" || got.SourceItemID == "" ||
			got.OriginalURL == "" || got.Title == "" || got.PublishedAt == "" || got.RetrievedAt == "" ||
			got.CreatedAt == "" || got.UpdatedAt == "" || got.CreatedBy != "rss-ingestion" ||
			got.UpdatedBy != "rss-ingestion" || got.Provider != "openai" || got.Model != "embedding-model" {
			t.Errorf("source_records[%d] = %#v, want complete canonical match %q", index, got, wantID)
		}
	}
	if output.SourceRecords[0].CosineDistance != 0 || output.SourceRecords[1].CosineDistance != 0 ||
		output.SourceRecords[2].CosineDistance <= 0 || output.SourceRecords[2].CosineDistance >= 1 ||
		output.SourceRecords[3].CosineDistance != 1 {
		t.Errorf("distances = [%v %v %v %v], want ordered [0 0 between-0-and-1 1]",
			output.SourceRecords[0].CosineDistance,
			output.SourceRecords[1].CosineDistance,
			output.SourceRecords[2].CosineDistance,
			output.SourceRecords[3].CosineDistance,
		)
	}

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

type economicEventContextIntegrationOutput struct {
	Event struct {
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
	} `json:"event"`
	PublicationWindowStart string `json:"publication_window_start"`
	PublicationWindowEnd   string `json:"publication_window_end"`
	Observations           []struct {
		ID                  string  `json:"id"`
		EconomicEventID     string  `json:"economic_event_id"`
		Source              string  `json:"source"`
		SourceObservationID string  `json:"source_observation_id"`
		SourceURL           string  `json:"source_url"`
		ObservedAt          string  `json:"observed_at"`
		Consensus           *string `json:"consensus"`
		Previous            *string `json:"previous"`
		Actual              *string `json:"actual"`
		CreatedAt           string  `json:"created_at"`
		UpdatedAt           string  `json:"updated_at"`
		CreatedBy           string  `json:"created_by"`
		UpdatedBy           string  `json:"updated_by"`
	} `json:"observations"`
	SourceRecords []struct {
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
	} `json:"source_records"`
}
