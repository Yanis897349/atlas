package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseUnlinkWatchlistEventCommandNormalizesInput(t *testing.T) {
	command, err := parseUnlinkWatchlistEventCommand([]string{
		"--symbol", " eurUsd ",
		"--event-id", "00000000-0000-0000-0000-000000000002",
		"--id", "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("parseUnlinkWatchlistEventCommand() error = %v", err)
	}
	want := unlinkWatchlistEventCommand{
		watchlistID: "00000000-0000-0000-0000-000000000001",
		symbol:      "EURUSD",
		eventID:     "00000000-0000-0000-0000-000000000002",
	}
	if !reflect.DeepEqual(command, want) {
		t.Errorf("command = %#v, want %#v", command, want)
	}
}

func TestRunRejectsInvalidUnlinkWatchlistEventArgumentsBeforeDatabaseSetup(t *testing.T) {
	valid := validUnlinkWatchlistEventArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing ID", arguments: withoutFlag(valid, "--id"), contains: "--id is required"},
		{name: "missing symbol", arguments: withoutFlag(valid, "--symbol"), contains: "--symbol is required"},
		{name: "missing event ID", arguments: withoutFlag(valid, "--event-id"), contains: "--event-id is required"},
		{name: "malformed ID", arguments: replaceFlag(valid, "--id", "bad"), contains: "--id must be a UUID"},
		{name: "malformed event ID", arguments: replaceFlag(valid, "--event-id", "bad"), contains: "--event-id must be a UUID"},
		{name: "blank symbol", arguments: replaceFlag(valid, "--symbol", " "), contains: "--symbol must not be blank"},
		{name: "repeated flag", arguments: append(valid, "--symbol", "DXY"), contains: "must only be provided once"},
		{name: "unknown flag", arguments: append(valid, "--format", "json"), contains: "flag provided but not defined"},
		{name: "positional", arguments: append(valid, "extra"), contains: "unexpected positional arguments"},
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

func TestRunUnlinkWatchlistEventCallsRepository(t *testing.T) {
	repository := &eventLinkRepositoryStub{}
	command := unlinkWatchlistEventCommand{
		watchlistID: "00000000-0000-0000-0000-000000000001",
		symbol:      "SPY",
		eventID:     "00000000-0000-0000-0000-000000000002",
	}

	if err := runUnlinkWatchlistEvent(t.Context(), repository, command); err != nil {
		t.Fatalf("runUnlinkWatchlistEvent() error = %v", err)
	}
	if repository.deleteCalls != 1 || repository.watchlistID != command.watchlistID ||
		repository.symbol != command.symbol || repository.eventID != command.eventID {
		t.Errorf("repository deletion = %#v, want complete command", repository)
	}
}

func TestRunUnlinkWatchlistEventPreservesFailures(t *testing.T) {
	wantErr := errors.New("event unlinking unavailable")
	for _, err := range []error{context.Canceled, wantErr} {
		repository := &eventLinkRepositoryStub{err: err}
		got := runUnlinkWatchlistEvent(t.Context(), repository, unlinkWatchlistEventCommand{})
		if got == nil || !strings.Contains(got.Error(), "unlink watchlist event") {
			t.Fatalf("runUnlinkWatchlistEvent() error = %v, want contextual failure", got)
		}
		if !errors.Is(got, err) {
			t.Fatalf("runUnlinkWatchlistEvent() error = %v, want wrapped %v", got, err)
		}
	}
}

func validUnlinkWatchlistEventArguments() []string {
	return []string{
		"unlink-watchlist-event",
		"--id", "00000000-0000-0000-0000-000000000001",
		"--symbol", "SPY",
		"--event-id", "00000000-0000-0000-0000-000000000002",
	}
}
