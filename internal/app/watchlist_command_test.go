package app

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

	"github.com/Yanis897349/atlas/internal/watchlist"
)

func TestParseCreateWatchlistCommandNormalizesOrderedInput(t *testing.T) {
	command, err := parseCreateWatchlistCommand([]string{
		"--symbol", " eurusd ",
		"--actor", " analyst ",
		"--name", " Macro & rates ",
		"--symbol", "SpY",
		"--symbol", "brk.b",
	})
	if err != nil {
		t.Fatalf("parseCreateWatchlistCommand() error = %v", err)
	}
	want := createWatchlistCommand{
		definition: watchlist.Definition{Name: "Macro & rates", Symbols: []string{"EURUSD", "SPY", "BRK.B"}},
		actor:      "analyst",
	}
	if !reflect.DeepEqual(command, want) {
		t.Errorf("parseCreateWatchlistCommand() = %#v, want %#v", command, want)
	}
}

func TestRunRejectsInvalidWatchlistArgumentsBeforeDatabaseSetup(t *testing.T) {
	validCreate := validCreateWatchlistArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing name", arguments: withoutFlag(validCreate, "--name"), contains: "--name is required"},
		{name: "missing actor", arguments: withoutFlag(validCreate, "--actor"), contains: "--actor is required"},
		{name: "missing symbol", arguments: withoutFlag(validCreate, "--symbol"), contains: "--symbol is required"},
		{name: "blank name", arguments: replaceFlag(validCreate, "--name", " "), contains: "--name must not be blank"},
		{name: "blank actor", arguments: replaceFlag(validCreate, "--actor", " "), contains: "--actor must not be blank"},
		{name: "blank symbol", arguments: replaceFlag(validCreate, "--symbol", " "), contains: "--symbol 1 must not be blank"},
		{name: "normalized duplicate symbol", arguments: append(append([]string(nil), validCreate...), "--symbol", " spy "), contains: "--symbol \"SPY\" is duplicated"},
		{name: "create unknown flag", arguments: append(append([]string(nil), validCreate...), "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "create positional argument", arguments: append(append([]string(nil), validCreate...), "extra"), contains: "unexpected positional arguments"},
		{name: "lookup missing ID", arguments: []string{"watchlist"}, contains: "--id is required"},
		{name: "lookup malformed ID", arguments: []string{"watchlist", "--id", "not-a-uuid"}, contains: "--id must be a UUID"},
		{name: "lookup repeated ID", arguments: []string{"watchlist", "--id", "00000000-0000-0000-0000-000000000001", "--id", "00000000-0000-0000-0000-000000000002"}, contains: "must only be provided once"},
		{name: "lookup unknown flag", arguments: []string{"watchlist", "--id", "00000000-0000-0000-0000-000000000001", "--format", "yaml"}, contains: "flag provided but not defined"},
		{name: "lookup positional argument", arguments: []string{"watchlist", "--id", "00000000-0000-0000-0000-000000000001", "extra"}, contains: "unexpected positional arguments"},
		{name: "missing limit", arguments: []string{"watchlists"}, contains: "--limit is required"},
		{name: "zero limit", arguments: []string{"watchlists", "--limit", "0"}, contains: "--limit must be between 1 and 100"},
		{name: "limit above maximum", arguments: []string{"watchlists", "--limit", "101"}, contains: "--limit must be between 1 and 100"},
		{name: "list unknown flag", arguments: []string{"watchlists", "--limit", "10", "--format", "yaml"}, contains: "flag provided but not defined"},
		{name: "list positional argument", arguments: []string{"watchlists", "--limit", "10", "extra"}, contains: "unexpected positional arguments"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Run(t.Context(), test.arguments, Dependencies{Getenv: func(string) string {
				t.Fatal("configuration read for invalid command arguments")
				return ""
			}})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run() error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestParseWatchlistsQuery(t *testing.T) {
	query, err := parseWatchlistsQuery([]string{"--limit", "12"})
	if err != nil {
		t.Fatalf("parseWatchlistsQuery() error = %v", err)
	}
	if query.limit != 12 {
		t.Errorf("query limit = %d, want 12", query.limit)
	}
}

func TestParseWatchlistQuery(t *testing.T) {
	query, err := parseWatchlistQuery([]string{"--id", "00000000-0000-0000-0000-000000000012"})
	if err != nil {
		t.Fatalf("parseWatchlistQuery() error = %v", err)
	}
	if query.id != "00000000-0000-0000-0000-000000000012" {
		t.Errorf("query ID = %q, want supplied UUID", query.id)
	}
}

func TestRunCreateWatchlistWritesCompleteJSON(t *testing.T) {
	repository := &watchlistRepositoryStub{}
	stdout := &bytes.Buffer{}
	command := validCreateWatchlistCommand()

	if err := runCreateWatchlist(t.Context(), repository, stdout, command); err != nil {
		t.Fatalf("runCreateWatchlist() error = %v", err)
	}

	var output watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if repository.createCalls != 1 || !reflect.DeepEqual(repository.definition, command.definition) || repository.actor != command.actor {
		t.Errorf("repository creation = (%d, %#v, %q), want one complete call", repository.createCalls, repository.definition, repository.actor)
	}
	if output.ID != "00000000-0000-0000-0000-000000000001" || output.Name != "Macro & rates" ||
		!reflect.DeepEqual(output.Symbols, []string{"EURUSD", "SPY"}) || output.CreatedBy != "analyst" ||
		output.UpdatedBy != "analyst" || output.CreatedAt != "2026-07-12T08:00:00.123456789Z" ||
		output.UpdatedAt != "2026-07-12T08:00:00.123456789Z" {
		t.Errorf("output = %#v, want complete canonical watchlist", output)
	}
	if !strings.Contains(stdout.String(), "Macro & rates") {
		t.Errorf("stdout = %q, want unescaped content", stdout.String())
	}
}

func TestRunWatchlistsPreservesRepositoryOrderAndWritesEmptyArray(t *testing.T) {
	first := storedWatchlistFixture()
	second := first
	second.ID = "00000000-0000-0000-0000-000000000002"
	second.Name = "Second"
	repository := &watchlistRepositoryStub{watchlists: []watchlist.StoredWatchlist{first, second}}
	stdout := &bytes.Buffer{}

	if err := runWatchlists(t.Context(), repository, stdout, watchlistsQuery{limit: 2}); err != nil {
		t.Fatalf("runWatchlists() error = %v", err)
	}
	var output []watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if repository.listCalls != 1 || repository.limit != 2 || len(output) != 2 ||
		output[0].ID != first.ID || output[1].ID != second.ID {
		t.Errorf("listed output = %#v after (%d, %d), want repository order", output, repository.listCalls, repository.limit)
	}

	stdout.Reset()
	repository.watchlists = nil
	if err := runWatchlists(t.Context(), repository, stdout, watchlistsQuery{limit: 10}); err != nil {
		t.Fatalf("runWatchlists(empty) error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("stdout = %q, want empty JSON array", stdout.String())
	}
}

func TestRunWatchlistWritesCompleteJSON(t *testing.T) {
	repository := &watchlistRepositoryStub{watchlist: storedWatchlistFixture()}
	stdout := &bytes.Buffer{}
	query := watchlistQuery{id: "00000000-0000-0000-0000-000000000001"}

	if err := runWatchlist(t.Context(), repository, stdout, query); err != nil {
		t.Fatalf("runWatchlist() error = %v", err)
	}

	var output watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if repository.lookupCalls != 1 || repository.id != query.id {
		t.Errorf("repository lookup = (%d, %q), want (1, %q)", repository.lookupCalls, repository.id, query.id)
	}
	if output.ID != query.id || output.Name != "Macro & rates" ||
		!reflect.DeepEqual(output.Symbols, []string{"EURUSD", "SPY"}) || output.CreatedBy != "analyst" ||
		output.UpdatedBy != "analyst" || output.CreatedAt != "2026-07-12T08:00:00.123456789Z" ||
		output.UpdatedAt != "2026-07-12T08:00:00.123456789Z" {
		t.Errorf("output = %#v, want complete canonical watchlist", output)
	}
}

func TestRunWatchlistCommandsPreserveFailures(t *testing.T) {
	wantErr := errors.New("watchlists unavailable")
	tests := []struct {
		name     string
		run      func(watchlist.Persistence, watchlist.Reader, io.Writer) error
		err      error
		writer   io.Writer
		contains string
	}{
		{name: "create cancellation", err: context.Canceled, contains: "create watchlist", run: func(p watchlist.Persistence, _ watchlist.Reader, stdout io.Writer) error {
			return runCreateWatchlist(t.Context(), p, stdout, validCreateWatchlistCommand())
		}},
		{name: "create writer", writer: errorWriter{err: wantErr}, contains: "encode created watchlist", run: func(p watchlist.Persistence, _ watchlist.Reader, stdout io.Writer) error {
			return runCreateWatchlist(t.Context(), p, stdout, validCreateWatchlistCommand())
		}},
		{name: "list failure", err: wantErr, contains: "retrieve watchlists", run: func(_ watchlist.Persistence, r watchlist.Reader, stdout io.Writer) error {
			return runWatchlists(t.Context(), r, stdout, watchlistsQuery{limit: 10})
		}},
		{name: "list writer", writer: errorWriter{err: wantErr}, contains: "encode watchlists", run: func(_ watchlist.Persistence, r watchlist.Reader, stdout io.Writer) error {
			return runWatchlists(t.Context(), r, stdout, watchlistsQuery{limit: 10})
		}},
		{name: "lookup cancellation", err: context.Canceled, contains: "retrieve watchlist", run: func(_ watchlist.Persistence, r watchlist.Reader, stdout io.Writer) error {
			return runWatchlist(t.Context(), r, stdout, watchlistQuery{id: "00000000-0000-0000-0000-000000000001"})
		}},
		{name: "lookup writer", writer: errorWriter{err: wantErr}, contains: "encode watchlist", run: func(_ watchlist.Persistence, r watchlist.Reader, stdout io.Writer) error {
			return runWatchlist(t.Context(), r, stdout, watchlistQuery{id: "00000000-0000-0000-0000-000000000001"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &watchlistRepositoryStub{err: test.err}
			var stdout io.Writer = &bytes.Buffer{}
			if test.writer != nil {
				stdout = test.writer
			}
			err := test.run(repository, repository, stdout)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("command error = %v, want context %q", err, test.contains)
			}
			wrapped := test.err
			if wrapped == nil {
				wrapped = wantErr
			}
			if !errors.Is(err, wrapped) {
				t.Fatalf("command error = %v, want wrapped %v", err, wrapped)
			}
		})
	}
}

func validCreateWatchlistArguments() []string {
	return []string{"create-watchlist", "--name", "Macro", "--actor", "analyst", "--symbol", "SPY"}
}

func validCreateWatchlistCommand() createWatchlistCommand {
	return createWatchlistCommand{
		definition: watchlist.Definition{Name: "Macro & rates", Symbols: []string{"EURUSD", "SPY"}},
		actor:      "analyst",
	}
}

func storedWatchlistFixture() watchlist.StoredWatchlist {
	createdAt := time.Date(2026, time.July, 12, 10, 0, 0, 123456789, time.FixedZone("CEST", 2*60*60))
	return watchlist.StoredWatchlist{
		ID: "00000000-0000-0000-0000-000000000001",
		Definition: watchlist.Definition{
			Name: "Macro & rates", Symbols: []string{"EURUSD", "SPY"},
		},
		CreatedAt: createdAt, UpdatedAt: createdAt, CreatedBy: "analyst", UpdatedBy: "analyst",
	}
}

type watchlistRepositoryStub struct {
	watchlist   watchlist.StoredWatchlist
	watchlists  []watchlist.StoredWatchlist
	err         error
	definition  watchlist.Definition
	actor       string
	id          string
	limit       int
	createCalls int
	updateCalls int
	deleteCalls int
	lookupCalls int
	listCalls   int
}

func (repository *watchlistRepositoryStub) CreateWatchlist(
	_ context.Context,
	definition watchlist.Definition,
	actor string,
) (watchlist.StoredWatchlist, error) {
	repository.createCalls++
	repository.definition = definition
	repository.actor = actor
	if repository.err != nil {
		return watchlist.StoredWatchlist{}, repository.err
	}
	stored := storedWatchlistFixture()
	stored.Definition = definition
	stored.CreatedBy = actor
	stored.UpdatedBy = actor
	return stored, nil
}

func (repository *watchlistRepositoryStub) UpdateWatchlist(
	_ context.Context,
	id string,
	definition watchlist.Definition,
	actor string,
) (watchlist.StoredWatchlist, error) {
	repository.updateCalls++
	repository.id = id
	repository.definition = definition
	repository.actor = actor
	if repository.err != nil {
		return watchlist.StoredWatchlist{}, repository.err
	}
	stored := storedWatchlistFixture()
	stored.ID = id
	stored.Definition = definition
	stored.UpdatedAt = stored.UpdatedAt.Add(time.Hour)
	stored.UpdatedBy = actor
	return stored, nil
}

func (repository *watchlistRepositoryStub) DeleteWatchlist(_ context.Context, id string) error {
	repository.deleteCalls++
	repository.id = id
	return repository.err
}

func (repository *watchlistRepositoryStub) Watchlist(_ context.Context, id string) (watchlist.StoredWatchlist, error) {
	repository.lookupCalls++
	repository.id = id
	return repository.watchlist, repository.err
}

func (repository *watchlistRepositoryStub) Watchlists(_ context.Context, limit int) ([]watchlist.StoredWatchlist, error) {
	repository.listCalls++
	repository.limit = limit
	return repository.watchlists, repository.err
}
