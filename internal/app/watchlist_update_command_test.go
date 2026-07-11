package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

func TestParseUpdateWatchlistCommandNormalizesOrderedInput(t *testing.T) {
	command, err := parseUpdateWatchlistCommand([]string{
		"--symbol", " eurusd ",
		"--actor", " editor ",
		"--id", "00000000-0000-0000-0000-000000000012",
		"--name", " Updated focus ",
		"--symbol", "SpY",
		"--symbol", "brk.b",
	})
	if err != nil {
		t.Fatalf("parseUpdateWatchlistCommand() error = %v", err)
	}
	want := updateWatchlistCommand{
		id:         "00000000-0000-0000-0000-000000000012",
		definition: watchlist.Definition{Name: "Updated focus", Symbols: []string{"EURUSD", "SPY", "BRK.B"}},
		actor:      "editor",
	}
	if !reflect.DeepEqual(command, want) {
		t.Errorf("parseUpdateWatchlistCommand() = %#v, want %#v", command, want)
	}
}

func TestRunRejectsInvalidUpdateWatchlistArgumentsBeforeDatabaseSetup(t *testing.T) {
	valid := validUpdateWatchlistArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing ID", arguments: withoutFlag(valid, "--id"), contains: "--id is required"},
		{name: "malformed ID", arguments: replaceFlag(valid, "--id", "not-a-uuid"), contains: "--id must be a UUID"},
		{name: "repeated ID", arguments: append(append([]string(nil), valid...), "--id", "00000000-0000-0000-0000-000000000002"), contains: "must only be provided once"},
		{name: "missing name", arguments: withoutFlag(valid, "--name"), contains: "--name is required"},
		{name: "missing actor", arguments: withoutFlag(valid, "--actor"), contains: "--actor is required"},
		{name: "missing symbol", arguments: withoutFlag(valid, "--symbol"), contains: "--symbol is required"},
		{name: "blank name", arguments: replaceFlag(valid, "--name", " "), contains: "--name must not be blank"},
		{name: "blank actor", arguments: replaceFlag(valid, "--actor", " "), contains: "--actor must not be blank"},
		{name: "blank symbol", arguments: replaceFlag(valid, "--symbol", " "), contains: "--symbol 1 must not be blank"},
		{name: "normalized duplicate symbol", arguments: append(append([]string(nil), valid...), "--symbol", " spy "), contains: "--symbol \"SPY\" is duplicated"},
		{name: "unknown flag", arguments: append(append([]string(nil), valid...), "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(append([]string(nil), valid...), "extra"), contains: "unexpected positional arguments"},
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

func TestRunUpdateWatchlistWritesCompleteJSON(t *testing.T) {
	repository := &watchlistRepositoryStub{}
	stdout := &bytes.Buffer{}
	command := validUpdateWatchlistCommand()

	if err := runUpdateWatchlist(t.Context(), repository, stdout, command); err != nil {
		t.Fatalf("runUpdateWatchlist() error = %v", err)
	}

	var output watchlistOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if repository.updateCalls != 1 || repository.id != command.id ||
		!reflect.DeepEqual(repository.definition, command.definition) || repository.actor != command.actor {
		t.Errorf(
			"repository update = (%d, %q, %#v, %q), want one complete call",
			repository.updateCalls,
			repository.id,
			repository.definition,
			repository.actor,
		)
	}
	if output.ID != command.id || output.Name != "Updated focus" ||
		!reflect.DeepEqual(output.Symbols, []string{"DXY", "BRK.B"}) || output.CreatedBy != "analyst" ||
		output.UpdatedBy != "editor" || output.CreatedAt != "2026-07-12T08:00:00.123456789Z" ||
		output.UpdatedAt != "2026-07-12T09:00:00.123456789Z" {
		t.Errorf("output = %#v, want complete updated watchlist", output)
	}
}

func TestRunUpdateWatchlistPreservesFailuresWithoutJSON(t *testing.T) {
	wantErr := errors.New("update unavailable")
	tests := []struct {
		name   string
		err    error
		writer bool
	}{
		{name: "cancellation", err: context.Canceled},
		{name: "repository failure", err: wantErr},
		{name: "writer failure", writer: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &watchlistRepositoryStub{err: test.err}
			stdout := &bytes.Buffer{}
			var output = interface{ Write([]byte) (int, error) }(stdout)
			if test.writer {
				output = errorWriter{err: wantErr}
			}
			err := runUpdateWatchlist(t.Context(), repository, output, validUpdateWatchlistCommand())
			if err == nil || !strings.Contains(err.Error(), "update") {
				t.Fatalf("runUpdateWatchlist() error = %v, want contextual failure", err)
			}
			wrapped := test.err
			if wrapped == nil {
				wrapped = wantErr
			}
			if !errors.Is(err, wrapped) {
				t.Fatalf("runUpdateWatchlist() error = %v, want wrapped %v", err, wrapped)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want no JSON", stdout.String())
			}
		})
	}
}

func validUpdateWatchlistArguments() []string {
	return []string{
		"update-watchlist",
		"--id", "00000000-0000-0000-0000-000000000001",
		"--name", "Updated",
		"--actor", "editor",
		"--symbol", "SPY",
	}
}

func validUpdateWatchlistCommand() updateWatchlistCommand {
	return updateWatchlistCommand{
		id:         "00000000-0000-0000-0000-000000000001",
		definition: watchlist.Definition{Name: "Updated focus", Symbols: []string{"DXY", "BRK.B"}},
		actor:      "editor",
	}
}
