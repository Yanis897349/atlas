package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseDeleteWatchlistCommand(t *testing.T) {
	command, err := parseDeleteWatchlistCommand([]string{
		"--id", "00000000-0000-0000-0000-000000000012",
	})
	if err != nil {
		t.Fatalf("parseDeleteWatchlistCommand() error = %v", err)
	}
	if command.id != "00000000-0000-0000-0000-000000000012" {
		t.Errorf("command ID = %q, want supplied UUID", command.id)
	}
}

func TestRunRejectsInvalidDeleteWatchlistArgumentsBeforeDatabaseSetup(t *testing.T) {
	valid := []string{"delete-watchlist", "--id", "00000000-0000-0000-0000-000000000001"}
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing ID", arguments: []string{"delete-watchlist"}, contains: "--id is required"},
		{name: "malformed ID", arguments: []string{"delete-watchlist", "--id", "not-a-uuid"}, contains: "--id must be a UUID"},
		{name: "repeated ID", arguments: append(append([]string(nil), valid...), "--id", "00000000-0000-0000-0000-000000000002"), contains: "must only be provided once"},
		{name: "unknown flag", arguments: append(append([]string(nil), valid...), "--format", "json"), contains: "flag provided but not defined"},
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

func TestRunDeleteWatchlistCallsRepository(t *testing.T) {
	repository := &watchlistDeleteStub{}
	command := deleteWatchlistCommand{id: "00000000-0000-0000-0000-000000000001"}

	if err := runDeleteWatchlist(t.Context(), repository, command); err != nil {
		t.Fatalf("runDeleteWatchlist() error = %v", err)
	}
	if repository.deleteCalls != 1 || repository.id != command.id {
		t.Errorf("repository deletion = (%d, %q), want (1, %q)", repository.deleteCalls, repository.id, command.id)
	}
}

func TestRunDeleteWatchlistPreservesFailures(t *testing.T) {
	wantErr := errors.New("deletion unavailable")
	for _, err := range []error{context.Canceled, wantErr} {
		repository := &watchlistDeleteStub{err: err}
		got := runDeleteWatchlist(t.Context(), repository, deleteWatchlistCommand{
			id: "00000000-0000-0000-0000-000000000001",
		})
		if got == nil || !strings.Contains(got.Error(), "delete watchlist") {
			t.Fatalf("runDeleteWatchlist() error = %v, want contextual failure", got)
		}
		if !errors.Is(got, err) {
			t.Fatalf("runDeleteWatchlist() error = %v, want wrapped %v", got, err)
		}
	}
}

type watchlistDeleteStub struct {
	err         error
	id          string
	deleteCalls int
}

func (repository *watchlistDeleteStub) DeleteWatchlist(_ context.Context, id string) error {
	repository.deleteCalls++
	repository.id = id
	return repository.err
}
