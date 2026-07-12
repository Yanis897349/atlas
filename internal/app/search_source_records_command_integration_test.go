package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
)

func TestRunSearchesSourceRecordsEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	env := map[string]string{
		"ATLAS_DATABASE_URL":           database.URL,
		"ATLAS_OPENAI_API_KEY":         "search-secret",
		"ATLAS_OPENAI_EMBEDDING_MODEL": "embedding-model",
	}
	dependencies := Dependencies{Getenv: func(name string) string { return env[name] }}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	sourceRepository, err := ingestionpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(source records) error = %v", err)
	}
	publishedAt := time.Date(2026, time.July, 12, 14, 0, 0, 123000000, time.FixedZone("Paris", 2*60*60))
	records := make([]ingestion.StoredSourceRecord, 0, 3)
	for index, title := range []string{"Exact B", "Orthogonal", "Exact A"} {
		record, persistErr := sourceRepository.UpsertSourceRecord(t.Context(), ingestion.SourceRecord{
			Source:       "test-publisher",
			SourceItemID: []string{"exact-b", "orthogonal", "exact-a"}[index],
			OriginalURL:  "https://example.com/news/" + []string{"exact-b", "orthogonal", "exact-a"}[index],
			Title:        title,
			PublishedAt:  publishedAt.Add(time.Duration(index) * time.Minute),
			RetrievedAt:  publishedAt.Add(time.Duration(index+1) * time.Minute),
		}, "rss-ingestion")
		if persistErr != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", title, persistErr)
		}
		records = append(records, record)
	}

	embeddingRepository, err := searchpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(embeddings) error = %v", err)
	}
	if err := embeddingRepository.PersistSourceRecordEmbeddings(t.Context(), []search.SourceRecordEmbedding{
		{SourceRecordID: records[0].ID, Provider: "openai", Model: "embedding-model", Vector: []float32{1, 0}},
		{SourceRecordID: records[1].ID, Provider: "openai", Model: "embedding-model", Vector: []float32{0, 1}},
		{SourceRecordID: records[2].ID, Provider: "openai", Model: "embedding-model", Vector: []float32{1, 0}},
	}, "search-indexer"); err != nil {
		t.Fatalf("PersistSourceRecordEmbeddings() error = %v", err)
	}

	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		providerCalls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("request = %s %s, want POST /v1/embeddings", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer search-secret" {
			t.Errorf("Authorization = %q, want command credential", request.Header.Get("Authorization"))
		}
		var providerRequest struct {
			Model          string   `json:"model"`
			Input          []string `json:"input"`
			EncodingFormat string   `json:"encoding_format"`
		}
		if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if !reflect.DeepEqual(providerRequest.Input, []string{"  exact semantic query  "}) ||
			providerRequest.EncodingFormat != "float" {
			t.Errorf("provider request = %#v, want exact query and float encoding", providerRequest)
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
	t.Cleanup(server.Close)
	dependencies.OpenAIHTTPClient = server.Client()
	dependencies.OpenAIEmbeddingsEndpoint = server.URL + "/v1/embeddings"
	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout

	arguments := []string{
		"search-source-records", "--query", "  exact semantic query  ", "--limit", "3",
	}
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(search-source-records) error = %v", err)
	}
	var output []searchedSourceRecordIntegrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	exactRecords := []ingestion.StoredSourceRecord{records[0], records[2]}
	sort.Slice(exactRecords, func(left, right int) bool { return exactRecords[left].ID < exactRecords[right].ID })
	wantIDs := []string{exactRecords[0].ID, exactRecords[1].ID, records[1].ID}
	if len(output) != 3 {
		t.Fatalf("output = %#v, want three matches", output)
	}
	for index := range output {
		if output[index].ID != wantIDs[index] || output[index].Source != "test-publisher" ||
			output[index].OriginalURL == "" || output[index].PublishedAt == "" ||
			output[index].CreatedAt == "" || output[index].CreatedBy != "rss-ingestion" ||
			output[index].Provider != "openai" || output[index].Model != "embedding-model" {
			t.Errorf("output[%d] = %#v, want complete canonical match %q", index, output[index], wantIDs[index])
		}
	}
	if output[0].CosineDistance != 0 || output[1].CosineDistance != 0 || output[2].CosineDistance != 1 {
		t.Errorf("distances = [%v %v %v], want [0 0 1]", output[0].CosineDistance, output[1].CosineDistance, output[2].CosineDistance)
	}

	stdout.Reset()
	env["ATLAS_OPENAI_EMBEDDING_MODEL"] = "unindexed-model"
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(search-source-records empty) error = %v", err)
	}
	if stdout.String() != "[]\n" || providerCalls.Load() != 2 {
		t.Errorf("empty search = %q with %d provider calls, want [] after second query embedding", stdout.String(), providerCalls.Load())
	}
}

type searchedSourceRecordIntegrationOutput struct {
	ID             string  `json:"id"`
	Source         string  `json:"source"`
	OriginalURL    string  `json:"original_url"`
	PublishedAt    string  `json:"published_at"`
	CreatedAt      string  `json:"created_at"`
	CreatedBy      string  `json:"created_by"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	CosineDistance float64 `json:"cosine_distance"`
}
