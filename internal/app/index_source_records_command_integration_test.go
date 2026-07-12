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
)

func TestRunIndexesSourceRecordsEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	baseEnv := map[string]string{
		"ATLAS_DATABASE_URL":           database.URL,
		"ATLAS_OPENAI_API_KEY":         "command-secret",
		"ATLAS_OPENAI_EMBEDDING_MODEL": "embedding-model",
	}
	dependencies := Dependencies{Getenv: func(name string) string { return baseEnv[name] }}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	repository, err := ingestionpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(source records) error = %v", err)
	}
	windowStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(2 * time.Hour)
	records := []ingestion.SourceRecord{
		indexCommandSourceRecord("before", "Before", windowStart.Add(-time.Microsecond)),
		indexCommandSourceRecord("start", "  Exact start title  ", windowStart),
		indexCommandSourceRecord("tie-b", "Tie title B", windowStart.Add(time.Hour)),
		indexCommandSourceRecord("tie-a", "Tie title A", windowStart.Add(time.Hour)),
		indexCommandSourceRecord("end", "Exact end title", windowEnd),
		indexCommandSourceRecord("after", "After", windowEnd.Add(time.Microsecond)),
	}
	storedByItemID := make(map[string]ingestion.StoredSourceRecord, len(records))
	for _, record := range records {
		stored, persistErr := repository.UpsertSourceRecord(t.Context(), record, "rss-ingestion")
		if persistErr != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", record.SourceItemID, persistErr)
		}
		storedByItemID[record.SourceItemID] = stored
	}
	tieIDs := []string{storedByItemID["tie-a"].ID, storedByItemID["tie-b"].ID}
	sort.Strings(tieIDs)
	expected := []ingestion.StoredSourceRecord{storedByItemID["end"]}
	for _, id := range tieIDs {
		if id == storedByItemID["tie-a"].ID {
			expected = append(expected, storedByItemID["tie-a"])
		} else {
			expected = append(expected, storedByItemID["tie-b"])
		}
	}
	expected = append(expected, storedByItemID["start"])
	expectedTitles := make([]string, len(expected))
	for index := range expected {
		expectedTitles[index] = expected[index].Title
	}

	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		call := int(providerCalls.Add(1))
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("request = %s %s, want POST /v1/embeddings", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer command-secret" {
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
		if providerRequest.Model != "embedding-model" || providerRequest.EncodingFormat != "float" ||
			!reflect.DeepEqual(providerRequest.Input, expectedTitles) {
			t.Errorf("provider request = %#v, want model, float encoding, and exact ordered titles %#v", providerRequest, expectedTitles)
		}

		data := make([]map[string]any, 0, len(expected))
		for index := len(expected) - 1; index >= 0; index-- {
			data = append(data, map[string]any{
				"object":    "embedding",
				"index":     index,
				"embedding": []float32{float32(call*10 + index + 1), 1, 1},
			})
		}
		response.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(response).Encode(map[string]any{
			"object": "list",
			"model":  "embedding-model",
			"data":   data,
		}); err != nil {
			t.Errorf("encode provider response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	dependencies.OpenAIHTTPClient = server.Client()
	dependencies.OpenAIEmbeddingsEndpoint = server.URL + "/v1/embeddings"
	arguments := []string{
		"index-source-records",
		"--from", "2026-07-12T10:00:00+02:00",
		"--to", "2026-07-12T10:00:00Z",
		"--limit", "4",
		"--actor", "first-indexer",
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(index-source-records) error = %v", err)
	}
	assertIndexedSourceRecordOutput(t, stdout.Bytes(), expected)
	assertCommandEmbedding(t, database, expected[0].ID, "[11,1,1]", "first-indexer", "first-indexer")
	assertCommandEmbeddingCount(t, database, len(expected))

	stdout.Reset()
	arguments[len(arguments)-1] = "second-indexer"
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(index-source-records retry) error = %v", err)
	}
	assertIndexedSourceRecordOutput(t, stdout.Bytes(), expected)
	assertCommandEmbedding(t, database, expected[0].ID, "[21,1,1]", "first-indexer", "second-indexer")
	assertCommandEmbeddingCount(t, database, len(expected))
	if providerCalls.Load() != 2 {
		t.Errorf("provider calls = %d, want 2", providerCalls.Load())
	}

	stdout.Reset()
	emptyArguments := []string{
		"index-source-records",
		"--from", "2027-01-01T00:00:00Z",
		"--to", "2027-01-02T00:00:00Z",
		"--limit", "10",
		"--actor", "empty-indexer",
	}
	if err := Run(t.Context(), emptyArguments, dependencies); err != nil {
		t.Fatalf("Run(index-source-records empty) error = %v", err)
	}
	if stdout.String() != "[]\n" || providerCalls.Load() != 2 {
		t.Errorf("empty result = %q with %d provider calls, want [] and no additional call", stdout.String(), providerCalls.Load())
	}
}

func indexCommandSourceRecord(sourceItemID, title string, publishedAt time.Time) ingestion.SourceRecord {
	return ingestion.SourceRecord{
		Source:       "test-source",
		SourceItemID: sourceItemID,
		OriginalURL:  "https://example.com/news/" + sourceItemID,
		Title:        title,
		PublishedAt:  publishedAt,
		RetrievedAt:  publishedAt.Add(time.Minute),
	}
}

func assertIndexedSourceRecordOutput(
	t *testing.T,
	encoded []byte,
	expected []ingestion.StoredSourceRecord,
) {
	t.Helper()
	var output []struct {
		SourceRecordID string `json:"source_record_id"`
		Provider       string `json:"provider"`
		Model          string `json:"model"`
		Dimension      int    `json:"dimension"`
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	if len(output) != len(expected) {
		t.Fatalf("output = %#v, want %d indexed records", output, len(expected))
	}
	var rawOutput []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &rawOutput); err != nil {
		t.Fatalf("decode raw command output: %v", err)
	}
	for index := range output {
		if output[index].SourceRecordID != expected[index].ID || output[index].Provider != "openai" ||
			output[index].Model != "embedding-model" || output[index].Dimension != 3 {
			t.Errorf("output[%d] = %#v, want record %q with OpenAI provenance and dimension 3", index, output[index], expected[index].ID)
		}
		if len(rawOutput[index]) != 4 {
			t.Errorf("output[%d] fields = %#v, want only source_record_id, provider, model, and dimension", index, rawOutput[index])
		}
	}
	if bytes.Contains(encoded, []byte("vector")) || bytes.Contains(encoded, []byte("[11,1,1]")) {
		t.Errorf("output exposes vector data: %s", encoded)
	}
}

func assertCommandEmbedding(
	t *testing.T,
	database postgrestest.Database,
	sourceRecordID string,
	wantVector string,
	wantCreatedBy string,
	wantUpdatedBy string,
) {
	t.Helper()
	var provider, model, vector, createdBy, updatedBy string
	err := database.Pool.QueryRow(t.Context(), `
SELECT provider, model, embedding::text, created_by, updated_by
FROM source_record_embeddings
WHERE source_record_id = $1
`, sourceRecordID).Scan(&provider, &model, &vector, &createdBy, &updatedBy)
	if err != nil {
		t.Fatalf("load command embedding: %v", err)
	}
	if provider != "openai" || model != "embedding-model" || vector != wantVector ||
		createdBy != wantCreatedBy || updatedBy != wantUpdatedBy {
		t.Errorf(
			"stored embedding = (%q, %q, %q, %q, %q), want OpenAI model, vector %q, and audit (%q, %q)",
			provider, model, vector, createdBy, updatedBy, wantVector, wantCreatedBy, wantUpdatedBy,
		)
	}
}

func assertCommandEmbeddingCount(t *testing.T, database postgrestest.Database, want int) {
	t.Helper()
	var count int
	if err := database.Pool.QueryRow(t.Context(), "SELECT count(*) FROM source_record_embeddings").Scan(&count); err != nil {
		t.Fatalf("count command embeddings: %v", err)
	}
	if count != want {
		t.Errorf("command embedding count = %d, want %d", count, want)
	}
}
