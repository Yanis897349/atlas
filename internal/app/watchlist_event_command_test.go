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

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/watchlist"
)

func TestParseLinkWatchlistEventCommandNormalizesInput(t *testing.T) {
	command, err := parseLinkWatchlistEventCommand([]string{
		"--symbol", " eurUsd ", "--actor", " analyst ",
		"--event-id", "00000000-0000-0000-0000-000000000002",
		"--id", "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("parseLinkWatchlistEventCommand() error = %v", err)
	}
	want := linkWatchlistEventCommand{
		watchlistID: "00000000-0000-0000-0000-000000000001",
		symbol:      "EURUSD", eventID: "00000000-0000-0000-0000-000000000002", actor: "analyst",
	}
	if !reflect.DeepEqual(command, want) {
		t.Errorf("command = %#v, want %#v", command, want)
	}
}

func TestParseWatchlistEventsQueryNormalizesInput(t *testing.T) {
	query, err := parseWatchlistEventsQuery([]string{
		"--limit", "12", "--symbol", " spy ", "--id", "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("parseWatchlistEventsQuery() error = %v", err)
	}
	want := watchlistEventsQuery{
		watchlistID: "00000000-0000-0000-0000-000000000001", symbol: "SPY", limit: 12,
	}
	if !reflect.DeepEqual(query, want) {
		t.Errorf("query = %#v, want %#v", query, want)
	}
}

func TestRunRejectsInvalidWatchlistEventArgumentsBeforeDatabaseSetup(t *testing.T) {
	validLink := validLinkWatchlistEventArguments()
	validList := validWatchlistEventsArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "link missing ID", arguments: withoutFlag(validLink, "--id"), contains: "--id is required"},
		{name: "link missing symbol", arguments: withoutFlag(validLink, "--symbol"), contains: "--symbol is required"},
		{name: "link missing event ID", arguments: withoutFlag(validLink, "--event-id"), contains: "--event-id is required"},
		{name: "link missing actor", arguments: withoutFlag(validLink, "--actor"), contains: "--actor is required"},
		{name: "link malformed ID", arguments: replaceFlag(validLink, "--id", "bad"), contains: "--id must be a UUID"},
		{name: "link malformed event ID", arguments: replaceFlag(validLink, "--event-id", "bad"), contains: "--event-id must be a UUID"},
		{name: "link blank symbol", arguments: replaceFlag(validLink, "--symbol", " "), contains: "--symbol must not be blank"},
		{name: "link blank actor", arguments: replaceFlag(validLink, "--actor", " "), contains: "--actor must not be blank"},
		{name: "link repeated flag", arguments: append(validLink, "--actor", "second"), contains: "must only be provided once"},
		{name: "link unknown flag", arguments: append(validLink, "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "link positional", arguments: append(validLink, "extra"), contains: "unexpected positional arguments"},
		{name: "list missing ID", arguments: withoutFlag(validList, "--id"), contains: "--id is required"},
		{name: "list missing symbol", arguments: withoutFlag(validList, "--symbol"), contains: "--symbol is required"},
		{name: "list missing limit", arguments: withoutFlag(validList, "--limit"), contains: "--limit is required"},
		{name: "list malformed ID", arguments: replaceFlag(validList, "--id", "bad"), contains: "--id must be a UUID"},
		{name: "list blank symbol", arguments: replaceFlag(validList, "--symbol", " "), contains: "--symbol must not be blank"},
		{name: "list nonnumeric limit", arguments: replaceFlag(validList, "--limit", "many"), contains: "--limit must be between 1 and 100"},
		{name: "list zero limit", arguments: replaceFlag(validList, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "list high limit", arguments: replaceFlag(validList, "--limit", "101"), contains: "--limit must be between 1 and 100"},
		{name: "list repeated flag", arguments: append(validList, "--limit", "2"), contains: "must only be provided once"},
		{name: "list unknown flag", arguments: append(validList, "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "list positional", arguments: append(validList, "extra"), contains: "unexpected positional arguments"},
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

func TestRunLinkWatchlistEventWritesCompleteJSON(t *testing.T) {
	stored := storedEventLinkFixture()
	repository := &eventLinkRepositoryStub{links: []watchlist.StoredEventLink{stored}}
	stdout := &bytes.Buffer{}
	command := linkWatchlistEventCommand{
		watchlistID: stored.WatchlistID, symbol: stored.Symbol, eventID: stored.Event.ID, actor: stored.CreatedBy,
	}
	if err := runLinkWatchlistEvent(t.Context(), repository, stdout, command); err != nil {
		t.Fatalf("runLinkWatchlistEvent() error = %v", err)
	}
	var output watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if repository.createCalls != 1 || repository.watchlistID != command.watchlistID ||
		repository.symbol != command.symbol || repository.eventID != command.eventID || repository.actor != command.actor {
		t.Errorf("repository creation = %#v, want complete command", repository)
	}
	if output.ID != stored.ID || output.WatchlistID != stored.WatchlistID || output.Symbol != stored.Symbol ||
		output.CreatedAt != "2026-07-12T08:00:00.123456789Z" || output.CreatedBy != "analyst" ||
		output.Event.ID != stored.Event.ID || output.Event.Source != "test-calendar" ||
		output.Event.SourceURL != "https://example.com/events/cpi" ||
		output.Event.ScheduledAt != "2026-07-15T10:30:00Z" || output.Event.CreatedBy != "calendar-user" {
		t.Errorf("output = %#v, want complete canonical link", output)
	}
}

func TestRunWatchlistEventsPreservesOrderAndWritesEmptyArray(t *testing.T) {
	first := storedEventLinkFixture()
	second := first
	second.ID = "00000000-0000-0000-0000-000000000004"
	second.Event.ID = "00000000-0000-0000-0000-000000000005"
	repository := &eventLinkRepositoryStub{links: []watchlist.StoredEventLink{first, second}}
	stdout := &bytes.Buffer{}
	query := watchlistEventsQuery{watchlistID: first.WatchlistID, symbol: first.Symbol, limit: 2}
	if err := runWatchlistEvents(t.Context(), repository, stdout, query); err != nil {
		t.Fatalf("runWatchlistEvents() error = %v", err)
	}
	var output []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if repository.listCalls != 1 || repository.limit != 2 || len(output) != 2 ||
		output[0].ID != first.ID || output[1].ID != second.ID {
		t.Errorf("output = %#v, want repository order", output)
	}
	repository.links = nil
	stdout.Reset()
	if err := runWatchlistEvents(t.Context(), repository, stdout, query); err != nil {
		t.Fatalf("runWatchlistEvents(empty) error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("stdout = %q, want empty JSON array", stdout.String())
	}
}

func TestRunWatchlistEventCommandsPreserveFailures(t *testing.T) {
	wantErr := errors.New("event links unavailable")
	tests := []struct {
		name     string
		err      error
		writer   io.Writer
		contains string
		invoke   func(*eventLinkRepositoryStub, io.Writer) error
	}{
		{name: "link cancellation", err: context.Canceled, contains: "link watchlist event", invoke: func(repository *eventLinkRepositoryStub, stdout io.Writer) error {
			return runLinkWatchlistEvent(t.Context(), repository, stdout, linkWatchlistEventCommand{})
		}},
		{name: "link duplicate", err: wantErr, contains: "link watchlist event", invoke: func(repository *eventLinkRepositoryStub, stdout io.Writer) error {
			return runLinkWatchlistEvent(t.Context(), repository, stdout, linkWatchlistEventCommand{})
		}},
		{name: "link writer", writer: errorWriter{err: wantErr}, contains: "encode linked watchlist event", invoke: func(repository *eventLinkRepositoryStub, stdout io.Writer) error {
			return runLinkWatchlistEvent(t.Context(), repository, stdout, linkWatchlistEventCommand{})
		}},
		{name: "list cancellation", err: context.Canceled, contains: "retrieve watchlist events", invoke: func(repository *eventLinkRepositoryStub, stdout io.Writer) error {
			return runWatchlistEvents(t.Context(), repository, stdout, watchlistEventsQuery{})
		}},
		{name: "list not found", err: wantErr, contains: "retrieve watchlist events", invoke: func(repository *eventLinkRepositoryStub, stdout io.Writer) error {
			return runWatchlistEvents(t.Context(), repository, stdout, watchlistEventsQuery{})
		}},
		{name: "list writer", writer: errorWriter{err: wantErr}, contains: "encode watchlist events", invoke: func(repository *eventLinkRepositoryStub, stdout io.Writer) error {
			return runWatchlistEvents(t.Context(), repository, stdout, watchlistEventsQuery{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &eventLinkRepositoryStub{links: []watchlist.StoredEventLink{storedEventLinkFixture()}, err: test.err}
			var stdout io.Writer = &bytes.Buffer{}
			if test.writer != nil {
				stdout = test.writer
			}
			err := test.invoke(repository, stdout)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want context %q", err, test.contains)
			}
			wrapped := test.err
			if wrapped == nil {
				wrapped = wantErr
			}
			if !errors.Is(err, wrapped) {
				t.Fatalf("error = %v, want wrapped %v", err, wrapped)
			}
		})
	}
}

func validLinkWatchlistEventArguments() []string {
	return []string{"link-watchlist-event", "--id", "00000000-0000-0000-0000-000000000001", "--symbol", "SPY", "--event-id", "00000000-0000-0000-0000-000000000002", "--actor", "analyst"}
}

func validWatchlistEventsArguments() []string {
	return []string{"watchlist-events", "--id", "00000000-0000-0000-0000-000000000001", "--symbol", "SPY", "--limit", "10"}
}

func storedEventLinkFixture() watchlist.StoredEventLink {
	linkTime := time.Date(2026, time.July, 12, 10, 0, 0, 123456789, time.FixedZone("CEST", 2*60*60))
	eventTime := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	return watchlist.StoredEventLink{
		ID:          "00000000-0000-0000-0000-000000000003",
		WatchlistID: "00000000-0000-0000-0000-000000000001", Symbol: "EURUSD",
		Event: calendar.StoredEvent{
			ID:        "00000000-0000-0000-0000-000000000002",
			Event:     calendar.Event{Source: "test-calendar", ExternalEventID: "cpi", Name: "CPI release", Region: calendar.RegionUnitedStates, Type: calendar.EventTypeInflation, ScheduledAt: eventTime, SourceURL: "https://example.com/events/cpi", RetrievedAt: eventTime.Add(-time.Hour)},
			CreatedAt: linkTime, UpdatedAt: linkTime, CreatedBy: "calendar-user", UpdatedBy: "calendar-user",
		},
		CreatedAt: linkTime, UpdatedAt: linkTime, CreatedBy: "analyst", UpdatedBy: "analyst",
	}
}

type eventLinkRepositoryStub struct {
	links       []watchlist.StoredEventLink
	err         error
	watchlistID string
	symbol      string
	eventID     string
	actor       string
	limit       int
	createCalls int
	listCalls   int
}

func (repository *eventLinkRepositoryStub) CreateEventLink(
	_ context.Context, watchlistID, symbol, eventID, actor string,
) (watchlist.StoredEventLink, error) {
	repository.createCalls++
	repository.watchlistID, repository.symbol = watchlistID, symbol
	repository.eventID, repository.actor = eventID, actor
	if repository.err != nil {
		return watchlist.StoredEventLink{}, repository.err
	}
	return repository.links[0], nil
}

func (repository *eventLinkRepositoryStub) EventLinks(
	_ context.Context, watchlistID, symbol string, limit int,
) ([]watchlist.StoredEventLink, error) {
	repository.listCalls++
	repository.watchlistID, repository.symbol, repository.limit = watchlistID, symbol, limit
	return repository.links, repository.err
}
