package searchcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/search"
)

func TestParseSearchSourceRecordsCommandPreservesExactQuery(t *testing.T) {
	command, recognized, err := Parse([]string{
		"search-source-records", "--limit", "24", "--query", "  central bank outlook  ",
	})
	if err != nil || !recognized {
		t.Fatalf("Parse() = (%#v, %t, %v), want recognized command", command, recognized, err)
	}
	if command.name != "search-source-records" || command.search.query != "  central bank outlook  " ||
		command.search.limit != 24 {
		t.Errorf("command = %#v, want exact query and normalized limit", command)
	}
}

func TestParseRejectsInvalidSearchSourceRecordsArguments(t *testing.T) {
	valid := validSearchSourceRecordsArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing query", arguments: withoutFlag(valid, "--query"), contains: "--query is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "blank query", arguments: replaceFlag(valid, "--query", " \t "), contains: "--query must not be blank"},
		{name: "nonnumeric limit", arguments: replaceFlag(valid, "--limit", "many"), contains: "--limit must be between 1 and 100"},
		{name: "zero limit", arguments: replaceFlag(valid, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "high limit", arguments: replaceFlag(valid, "--limit", "101"), contains: "--limit must be between 1 and 100"},
		{name: "repeated flag", arguments: append(valid, "--query", "second"), contains: "must only be provided once"},
		{name: "unknown flag", arguments: append(valid, "--source", "publisher"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(valid, "extra"), contains: "unexpected positional arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, recognized, err := Parse(test.arguments)
			if !recognized {
				t.Fatal("Parse() did not recognize search command")
			}
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				!strings.Contains(err.Error(), searchSourceRecordsUsage) {
				t.Fatalf("Parse() error = %v, want containing %q and usage", err, test.contains)
			}
		})
	}
}

func TestRunSearchSourceRecordsWritesCompleteOrderedRecords(t *testing.T) {
	paris := time.FixedZone("Paris", 2*60*60)
	firstTime := time.Date(2026, time.July, 12, 14, 0, 0, 123000000, paris)
	reader := &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{
		searchedSourceRecordFixture("record-second", "Second", firstTime, 0.1),
		searchedSourceRecordFixture("record-first", "First", firstTime.Add(time.Hour), 0.3),
	}}
	embedder := &embedderStub{batch: search.EmbeddingBatch{
		Provider: " openai ", Model: " embedding-model ",
		Embeddings: []search.ProviderEmbedding{{SourceRecordID: "semantic-search-query", Vector: []float32{1, 2}}},
	}}
	stdout := &bytes.Buffer{}
	command := searchSourceRecordsCommand{query: "  exact query  ", limit: 2}

	if err := runSearchSourceRecords(t.Context(), embedder, reader, stdout, command); err != nil {
		t.Fatalf("runSearchSourceRecords() error = %v", err)
	}
	if !reflect.DeepEqual(embedder.inputs, []search.EmbeddingInput{{
		SourceRecordID: "semantic-search-query", Text: "  exact query  ",
	}}) || reader.provider != "openai" || reader.model != "embedding-model" || reader.limit != 2 ||
		!reflect.DeepEqual(reader.vector, []float32{1, 2}) {
		t.Errorf("orchestration = embedder %#v reader %#v, want exact query and normalized provenance", embedder, reader)
	}
	var output []searchedSourceRecordOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output) != 2 || output[0].ID != "record-second" || output[1].ID != "record-first" ||
		output[0].PublishedAt != "2026-07-12T12:00:00.123Z" || output[0].CreatedBy != "ingestion" ||
		output[0].UpdatedBy != "refresh" || output[0].Provider != "openai" ||
		output[0].Model != "embedding-model" || output[0].CosineDistance != 0.1 {
		t.Errorf("output = %#v, want complete ordered canonical records", output)
	}
}

func TestRunSearchSourceRecordsWritesEmptyArray(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runSearchSourceRecords(
		t.Context(),
		&embedderStub{batch: validSearchQueryBatch()},
		&similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}},
		stdout,
		searchSourceRecordsCommand{query: "query", limit: 1},
	)
	if err != nil || stdout.String() != "[]\n" {
		t.Fatalf("runSearchSourceRecords() = (%q, %v), want empty JSON array", stdout.String(), err)
	}
}

func TestRunSearchSourceRecordsPreservesFailuresWithoutOutput(t *testing.T) {
	wantErr := errors.New("dependency unavailable")
	tests := []struct {
		name     string
		embedder search.Embedder
		reader   search.SimilarSourceRecordReader
		stdout   io.Writer
		contains string
		wantErr  error
	}{
		{name: "provider", embedder: &embedderStub{err: wantErr}, reader: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "embed semantic search query", wantErr: wantErr},
		{name: "cancellation", embedder: &embedderStub{err: context.Canceled}, reader: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "embed semantic search query", wantErr: context.Canceled},
		{name: "repository", embedder: &embedderStub{batch: validSearchQueryBatch()}, reader: &similarSourceRecordReaderStub{err: wantErr}, stdout: &bytes.Buffer{}, contains: "retrieve similar source records", wantErr: wantErr},
		{name: "encoding", embedder: &embedderStub{batch: validSearchQueryBatch()}, reader: &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{{CosineDistance: math.NaN()}}}, stdout: &bytes.Buffer{}, contains: "encode searched source records", wantErr: nil},
		{name: "writer", embedder: &embedderStub{batch: validSearchQueryBatch()}, reader: &similarSourceRecordReaderStub{}, stdout: errorWriter{err: wantErr}, contains: "write searched source records", wantErr: wantErr},
		{name: "short writer", embedder: &embedderStub{batch: validSearchQueryBatch()}, reader: &similarSourceRecordReaderStub{}, stdout: shortWriter{}, contains: "short write", wantErr: io.ErrShortWrite},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runSearchSourceRecords(
				t.Context(), test.embedder, test.reader, test.stdout,
				searchSourceRecordsCommand{query: "query", limit: 1},
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				(test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Fatalf("error = %v, want contextual failure containing %q", err, test.contains)
			}
			if buffer, ok := test.stdout.(*bytes.Buffer); ok && buffer.Len() != 0 {
				t.Errorf("stdout = %q, want no JSON on failure", buffer.String())
			}
		})
	}
}

func validSearchSourceRecordsArguments() []string {
	return []string{"search-source-records", "--query", "inflation outlook", "--limit", "10"}
}

func validSearchQueryBatch() search.EmbeddingBatch {
	return search.EmbeddingBatch{
		Provider: "openai", Model: "model",
		Embeddings: []search.ProviderEmbedding{{SourceRecordID: "semantic-search-query", Vector: []float32{1}}},
	}
}

func searchedSourceRecordFixture(id, title string, timestamp time.Time, distance float64) search.SimilarSourceRecord {
	return search.SimilarSourceRecord{
		SourceRecord: ingestion.StoredSourceRecord{
			ID: id,
			SourceRecord: ingestion.SourceRecord{
				Source: "publisher", SourceItemID: "item-" + id,
				OriginalURL: "https://example.com/" + id, Title: title,
				PublishedAt: timestamp, RetrievedAt: timestamp.Add(time.Minute),
			},
			CreatedAt: timestamp.Add(2 * time.Minute), UpdatedAt: timestamp.Add(3 * time.Minute),
			CreatedBy: "ingestion", UpdatedBy: "refresh",
		},
		Provider: "openai", Model: "embedding-model", CosineDistance: distance,
	}
}

type similarSourceRecordReaderStub struct {
	results  []search.SimilarSourceRecord
	err      error
	provider string
	model    string
	vector   []float32
	limit    int
}

func (reader *similarSourceRecordReaderStub) SimilarSourceRecords(
	_ context.Context,
	provider string,
	model string,
	vector []float32,
	limit int,
) ([]search.SimilarSourceRecord, error) {
	reader.provider, reader.model, reader.vector, reader.limit = provider, model, append([]float32(nil), vector...), limit
	return reader.results, reader.err
}

type panicSimilarSourceRecordReader struct{}

func (panicSimilarSourceRecordReader) SimilarSourceRecords(
	context.Context, string, string, []float32, int,
) ([]search.SimilarSourceRecord, error) {
	panic("similar source record reader must not be called")
}
