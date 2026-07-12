package searchcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/search"
)

func TestParseIndexSourceRecordsCommandNormalizesInput(t *testing.T) {
	command, recognized, err := Parse([]string{
		"index-source-records",
		"--actor", " indexer ",
		"--to", "2026-07-12T14:00:00+02:00",
		"--limit", "24",
		"--from", "2026-07-12T08:00:00Z",
	})
	if err != nil || !recognized {
		t.Fatalf("Parse() = (%#v, %t, %v), want recognized command", command, recognized, err)
	}
	wantStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	if command.name != "index-source-records" || command.index.windowStart != wantStart ||
		command.index.windowEnd != wantEnd || command.index.limit != 24 || command.index.actor != "indexer" {
		t.Errorf("command = %#v, want normalized complete command", command)
	}
}

func TestParseIndexSourceRecordsCommandAcceptsEqualInclusiveWindow(t *testing.T) {
	arguments := validIndexSourceRecordsArguments()
	arguments = replaceFlag(arguments, "--to", "2026-07-12T08:00:00Z")
	if _, _, err := Parse(arguments); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsInvalidIndexSourceRecordsArguments(t *testing.T) {
	valid := validIndexSourceRecordsArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing from", arguments: withoutFlag(valid, "--from"), contains: "--from is required"},
		{name: "missing to", arguments: withoutFlag(valid, "--to"), contains: "--to is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "missing actor", arguments: withoutFlag(valid, "--actor"), contains: "--actor is required"},
		{name: "malformed from", arguments: replaceFlag(valid, "--from", "today"), contains: "--from must be RFC3339"},
		{name: "malformed to", arguments: replaceFlag(valid, "--to", "later"), contains: "--to must be RFC3339"},
		{name: "reversed window", arguments: replaceFlag(valid, "--to", "2026-07-12T07:59:59Z"), contains: "--to must not be before --from"},
		{name: "nonnumeric limit", arguments: replaceFlag(valid, "--limit", "many"), contains: "--limit must be between 1 and 100"},
		{name: "zero limit", arguments: replaceFlag(valid, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "high limit", arguments: replaceFlag(valid, "--limit", "101"), contains: "--limit must be between 1 and 100"},
		{name: "blank actor", arguments: replaceFlag(valid, "--actor", " "), contains: "--actor must not be blank"},
		{name: "repeated flag", arguments: append(valid, "--actor", "second"), contains: "must only be provided once"},
		{name: "unknown flag", arguments: append(valid, "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(valid, "extra"), contains: "unexpected positional arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, recognized, err := Parse(test.arguments)
			if !recognized {
				t.Fatal("Parse() did not recognize search command")
			}
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				!strings.Contains(err.Error(), indexSourceRecordsUsage) {
				t.Fatalf("Parse() error = %v, want containing %q and usage", err, test.contains)
			}
		})
	}
}

func TestParseDoesNotRecognizeOtherCommands(t *testing.T) {
	for _, arguments := range [][]string{nil, {"migrate"}, {"daily-brief"}} {
		command, recognized, err := Parse(arguments)
		if err != nil || recognized || !reflect.DeepEqual(command, Command{}) {
			t.Errorf("Parse(%q) = (%#v, %t, %v), want zero command, false, nil", arguments, command, recognized, err)
		}
	}
}

func TestRunIndexSourceRecordsWritesOrderedMetadataWithoutVectors(t *testing.T) {
	reader := &sourceRecordReaderStub{records: []ingestion.StoredSourceRecord{
		{ID: "record-second", SourceRecord: ingestion.SourceRecord{Title: "Second exact title"}},
		{ID: "record-first", SourceRecord: ingestion.SourceRecord{Title: "First exact title"}},
	}}
	embedder := &embedderStub{batch: search.EmbeddingBatch{
		Provider: "openai",
		Model:    "embedding-model",
		Embeddings: []search.ProviderEmbedding{
			{SourceRecordID: "record-second", Vector: []float32{1, 2, 3}},
			{SourceRecordID: "record-first", Vector: []float32{4, 5, 6}},
		},
	}}
	writer := &embeddingWriterStub{}
	stdout := &bytes.Buffer{}
	command := indexSourceRecordsCommand{
		windowStart: time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC),
		windowEnd:   time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC),
		limit:       2,
		actor:       "indexer",
	}

	if err := runIndexSourceRecords(t.Context(), reader, embedder, writer, stdout, command); err != nil {
		t.Fatalf("runIndexSourceRecords() error = %v", err)
	}
	if reader.calls != 1 || reader.windowStart != command.windowStart || reader.windowEnd != command.windowEnd ||
		reader.limit != command.limit || writer.calls != 1 || writer.actor != command.actor {
		t.Errorf("orchestration = reader %#v writer %#v, want complete command", reader, writer)
	}
	wantInputs := []search.EmbeddingInput{
		{SourceRecordID: "record-second", Text: "Second exact title"},
		{SourceRecordID: "record-first", Text: "First exact title"},
	}
	if !reflect.DeepEqual(embedder.inputs, wantInputs) {
		t.Errorf("embedder inputs = %#v, want %#v", embedder.inputs, wantInputs)
	}
	var output []indexedSourceRecordOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	wantOutput := []indexedSourceRecordOutput{
		{SourceRecordID: "record-second", Provider: "openai", Model: "embedding-model", Dimension: 3},
		{SourceRecordID: "record-first", Provider: "openai", Model: "embedding-model", Dimension: 3},
	}
	wantJSON := "[" +
		`{"source_record_id":"record-second","provider":"openai","model":"embedding-model","dimension":3},` +
		`{"source_record_id":"record-first","provider":"openai","model":"embedding-model","dimension":3}` +
		"]\n"
	if !reflect.DeepEqual(output, wantOutput) || stdout.String() != wantJSON {
		t.Errorf("output = %#v (%s), want metadata without vectors", output, stdout.String())
	}
}

func TestRunIndexSourceRecordsWritesEmptyArray(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runIndexSourceRecords(
		t.Context(),
		&sourceRecordReaderStub{records: []ingestion.StoredSourceRecord{}},
		panicEmbedder{},
		&embeddingWriterStub{},
		stdout,
		indexSourceRecordsCommand{limit: 1, actor: "indexer"},
	)
	if err != nil {
		t.Fatalf("runIndexSourceRecords() error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("stdout = %q, want empty JSON array", stdout.String())
	}
}

func TestRunIndexSourceRecordsPreservesFailuresWithoutOutput(t *testing.T) {
	wantErr := errors.New("dependency unavailable")
	records := []ingestion.StoredSourceRecord{{ID: "record-1", SourceRecord: ingestion.SourceRecord{Title: "Title"}}}
	validBatch := search.EmbeddingBatch{
		Provider: "openai", Model: "model",
		Embeddings: []search.ProviderEmbedding{{SourceRecordID: "record-1", Vector: []float32{1}}},
	}
	tests := []struct {
		name     string
		reader   *sourceRecordReaderStub
		embedder search.Embedder
		writer   *embeddingWriterStub
		stdout   io.Writer
		contains string
	}{
		{name: "retrieval", reader: &sourceRecordReaderStub{err: wantErr}, embedder: panicEmbedder{}, writer: &embeddingWriterStub{}, stdout: &bytes.Buffer{}, contains: "retrieve source records"},
		{name: "cancellation", reader: &sourceRecordReaderStub{err: context.Canceled}, embedder: panicEmbedder{}, writer: &embeddingWriterStub{}, stdout: &bytes.Buffer{}, contains: "retrieve source records"},
		{name: "provider", reader: &sourceRecordReaderStub{records: records}, embedder: &embedderStub{err: wantErr}, writer: &embeddingWriterStub{}, stdout: &bytes.Buffer{}, contains: "embed retrieved source records"},
		{name: "persistence", reader: &sourceRecordReaderStub{records: records}, embedder: &embedderStub{batch: validBatch}, writer: &embeddingWriterStub{err: wantErr}, stdout: &bytes.Buffer{}, contains: "persist indexed"},
		{name: "writer", reader: &sourceRecordReaderStub{records: records}, embedder: &embedderStub{batch: validBatch}, writer: &embeddingWriterStub{}, stdout: errorWriter{err: wantErr}, contains: "write indexed source records"},
		{name: "short writer", reader: &sourceRecordReaderStub{records: records}, embedder: &embedderStub{batch: validBatch}, writer: &embeddingWriterStub{}, stdout: shortWriter{}, contains: "short write"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runIndexSourceRecords(
				t.Context(), test.reader, test.embedder, test.writer, test.stdout,
				indexSourceRecordsCommand{limit: 1, actor: "indexer"},
			)
			wantWrapped := wantErr
			switch test.name {
			case "cancellation":
				wantWrapped = context.Canceled
			case "short writer":
				wantWrapped = io.ErrShortWrite
			}
			if !errors.Is(err, wantWrapped) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want contextual %v", err, wantWrapped)
			}
			if buffer, ok := test.stdout.(*bytes.Buffer); ok && buffer.Len() != 0 {
				t.Errorf("stdout = %q, want no JSON on failure", buffer.String())
			}
		})
	}
}

func validIndexSourceRecordsArguments() []string {
	return []string{
		"index-source-records",
		"--from", "2026-07-12T08:00:00Z",
		"--to", "2026-07-12T12:00:00Z",
		"--limit", "10",
		"--actor", "indexer",
	}
}

func withoutFlag(arguments []string, name string) []string {
	result := make([]string, 0, len(arguments)-2)
	for index := 0; index < len(arguments); index++ {
		if arguments[index] == name {
			index++
			continue
		}
		result = append(result, arguments[index])
	}
	return result
}

func replaceFlag(arguments []string, name, value string) []string {
	result := append([]string(nil), arguments...)
	for index := range result {
		if result[index] == name {
			result[index+1] = value
			return result
		}
	}
	return result
}

type sourceRecordReaderStub struct {
	records     []ingestion.StoredSourceRecord
	err         error
	calls       int
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (reader *sourceRecordReaderStub) RecentSourceRecords(
	_ context.Context,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]ingestion.StoredSourceRecord, error) {
	reader.calls++
	reader.windowStart, reader.windowEnd, reader.limit = windowStart, windowEnd, limit
	return reader.records, reader.err
}

type embedderStub struct {
	batch  search.EmbeddingBatch
	err    error
	inputs []search.EmbeddingInput
}

func (embedder *embedderStub) Embed(
	_ context.Context,
	inputs []search.EmbeddingInput,
) (search.EmbeddingBatch, error) {
	embedder.inputs = append([]search.EmbeddingInput(nil), inputs...)
	return embedder.batch, embedder.err
}

type panicEmbedder struct{}

func (panicEmbedder) Embed(context.Context, []search.EmbeddingInput) (search.EmbeddingBatch, error) {
	panic("embedder must not be called")
}

type embeddingWriterStub struct {
	calls      int
	embeddings []search.SourceRecordEmbedding
	actor      string
	err        error
}

func (writer *embeddingWriterStub) PersistSourceRecordEmbeddings(
	_ context.Context,
	embeddings []search.SourceRecordEmbedding,
	actor string,
) error {
	writer.calls++
	writer.embeddings = embeddings
	writer.actor = actor
	return writer.err
}

type errorWriter struct{ err error }

func (writer errorWriter) Write([]byte) (int, error) { return 0, writer.err }

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) { return len(value) - 1, nil }
