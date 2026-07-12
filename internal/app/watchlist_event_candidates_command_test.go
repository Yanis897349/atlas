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

func TestParseLinkWatchlistEventsCommandNormalizesInput(t *testing.T) {
	command, err := parseLinkWatchlistEventsCommand([]string{
		"--actor", " classifier ",
		"--to", "2026-08-01T14:00:00+02:00",
		"--limit", "24",
		"--id", "00000000-0000-0000-0000-000000000001",
		"--from", "2026-08-01T08:00:00Z",
	})
	if err != nil {
		t.Fatalf("parseLinkWatchlistEventsCommand() error = %v", err)
	}
	if command.watchlistID != "00000000-0000-0000-0000-000000000001" ||
		!command.windowStart.Equal(time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)) ||
		!command.windowEnd.Equal(time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)) ||
		command.limit != 24 || command.actor != "classifier" {
		t.Errorf("command = %#v, want normalized complete command", command)
	}
}

func TestRunRejectsInvalidLinkWatchlistEventsArgumentsBeforeDatabaseSetup(t *testing.T) {
	valid := validLinkWatchlistEventsArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing ID", arguments: withoutFlag(valid, "--id"), contains: "--id is required"},
		{name: "missing from", arguments: withoutFlag(valid, "--from"), contains: "--from is required"},
		{name: "missing to", arguments: withoutFlag(valid, "--to"), contains: "--to is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "missing actor", arguments: withoutFlag(valid, "--actor"), contains: "--actor is required"},
		{name: "malformed ID", arguments: replaceFlag(valid, "--id", "bad"), contains: "--id must be a UUID"},
		{name: "malformed from", arguments: replaceFlag(valid, "--from", "soon"), contains: "--from must be RFC3339"},
		{name: "malformed to", arguments: replaceFlag(valid, "--to", "later"), contains: "--to must be RFC3339"},
		{name: "reversed window", arguments: replaceFlag(valid, "--to", "2026-07-31T23:59:59Z"), contains: "--to must not be before --from"},
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

func TestRunLinkWatchlistEventsOrchestratesAndWritesCompleteOrderedJSON(t *testing.T) {
	first := storedEventLinkFixture()
	first.Symbol = "EURUSD"
	second := first
	second.ID = "00000000-0000-0000-0000-000000000004"
	second.Symbol = "SPY"
	candidates := &linkCandidateReaderStub{events: []calendar.StoredEvent{first.Event}}
	reader := &candidateWatchlistReaderStub{stored: watchlist.StoredWatchlist{
		ID: first.WatchlistID,
		Definition: watchlist.Definition{
			Name:    "Macro events",
			Symbols: []string{"EURUSD", "SPY"},
		},
	}}
	writer := &candidateEventLinkWriterStub{links: []watchlist.StoredEventLink{first, second}}
	stdout := &bytes.Buffer{}
	command := linkWatchlistEventsCommand{
		watchlistID: first.WatchlistID,
		windowStart: time.Date(2026, time.July, 15, 8, 0, 0, 0, time.UTC),
		windowEnd:   time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC),
		limit:       12,
		actor:       "classifier",
	}

	if err := runLinkWatchlistEvents(t.Context(), candidates, reader, writer, stdout, command); err != nil {
		t.Fatalf("runLinkWatchlistEvents() error = %v", err)
	}
	if candidates.calls != 1 || candidates.windowStart != command.windowStart ||
		candidates.windowEnd != command.windowEnd || candidates.limit != command.limit {
		t.Errorf("candidate retrieval = %#v, want complete command window and limit", candidates)
	}
	if reader.calls != 1 || reader.id != command.watchlistID || writer.calls != 1 ||
		writer.watchlistID != command.watchlistID || writer.actor != command.actor {
		t.Errorf("link orchestration = (%#v, %#v), want complete command", reader, writer)
	}
	wantClassifications := []watchlist.EventRelevance{
		{Symbol: "EURUSD", Event: first.Event, Relevant: true},
		{Symbol: "SPY", Event: first.Event, Relevant: true},
	}
	if !reflect.DeepEqual(writer.classifications, wantClassifications) {
		t.Errorf("classifications = %#v, want %#v", writer.classifications, wantClassifications)
	}
	var output []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output) != 2 || output[0].ID != first.ID || output[1].ID != second.ID ||
		output[0].Event.ID != first.Event.ID || output[0].Event.SourceURL != first.Event.SourceURL ||
		output[0].CreatedAt != "2026-07-12T08:00:00.123456789Z" ||
		output[0].Event.ScheduledAt != "2026-07-15T10:30:00Z" {
		t.Errorf("output = %#v, want complete links in writer order", output)
	}
}

func TestRunLinkWatchlistEventsWritesEmptyArray(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runLinkWatchlistEvents(
		t.Context(),
		&linkCandidateReaderStub{events: []calendar.StoredEvent{}},
		&candidateWatchlistReaderStub{stored: watchlist.StoredWatchlist{
			ID:         "00000000-0000-0000-0000-000000000001",
			Definition: watchlist.Definition{Symbols: []string{"SPY"}},
		}},
		&candidateEventLinkWriterStub{links: []watchlist.StoredEventLink{}},
		stdout,
		linkWatchlistEventsCommand{},
	)
	if err != nil {
		t.Fatalf("runLinkWatchlistEvents() error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("stdout = %q, want empty JSON array", stdout.String())
	}
}

func TestWatchlistDispatcherRoutesCandidateLinking(t *testing.T) {
	link := storedEventLinkFixture()
	candidates := &linkCandidateReaderStub{events: []calendar.StoredEvent{link.Event}}
	repository := &candidateDispatchRepository{
		reader: candidateWatchlistReaderStub{stored: watchlist.StoredWatchlist{
			ID:         link.WatchlistID,
			Definition: watchlist.Definition{Symbols: []string{"SPY"}},
		}},
		writer: candidateEventLinkWriterStub{links: []watchlist.StoredEventLink{link}},
	}
	stdout := &bytes.Buffer{}
	command := watchlistCommand{
		name: "link-watchlist-events",
		linkEvents: linkWatchlistEventsCommand{
			watchlistID: link.WatchlistID,
			windowStart: link.Event.ScheduledAt.Add(-time.Hour),
			windowEnd:   link.Event.ScheduledAt.Add(time.Hour),
			limit:       10,
			actor:       "classifier",
		},
	}

	if err := runWatchlistCommand(t.Context(), repository, candidates, stdout, command); err != nil {
		t.Fatalf("runWatchlistCommand() error = %v", err)
	}
	if candidates.calls != 1 || repository.reader.calls != 1 || repository.writer.calls != 1 {
		t.Errorf("dependency calls = (%d, %d, %d), want (1, 1, 1)",
			candidates.calls, repository.reader.calls, repository.writer.calls)
	}
	var output []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output) != 1 || output[0].ID != link.ID {
		t.Errorf("output = %#v, want dispatched link %q", output, link.ID)
	}
}

func TestWatchlistDispatcherPreservesCandidateLinkingFailures(t *testing.T) {
	wantErr := errors.New("candidate repository unavailable")
	repository := &candidateDispatchRepository{}
	err := runWatchlistCommand(
		t.Context(),
		repository,
		&linkCandidateReaderStub{err: wantErr},
		&bytes.Buffer{},
		watchlistCommand{name: "link-watchlist-events"},
	)
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "link watchlist event candidates") ||
		!strings.Contains(err.Error(), "retrieve watchlist event candidates") {
		t.Fatalf("runWatchlistCommand() error = %v, want contextual %v", err, wantErr)
	}
	if repository.reader.calls != 0 || repository.writer.calls != 0 {
		t.Errorf("downstream calls = (%d, %d), want (0, 0)", repository.reader.calls, repository.writer.calls)
	}
}

func TestRunLinkWatchlistEventsPreservesFailures(t *testing.T) {
	wantErr := errors.New("candidate links unavailable")
	validReader := &candidateWatchlistReaderStub{stored: watchlist.StoredWatchlist{
		ID:         "00000000-0000-0000-0000-000000000001",
		Definition: watchlist.Definition{Symbols: []string{"SPY"}},
	}}
	validCandidates := &linkCandidateReaderStub{events: []calendar.StoredEvent{storedEventLinkFixture().Event}}
	tests := []struct {
		name       string
		candidates watchlist.EventCandidateReader
		reader     watchlist.WatchlistReader
		writer     watchlist.EventLinkWriter
		stdout     io.Writer
		contains   string
	}{
		{name: "candidate retrieval", candidates: &linkCandidateReaderStub{err: wantErr}, reader: validReader, writer: &candidateEventLinkWriterStub{}, stdout: &bytes.Buffer{}, contains: "retrieve watchlist event candidates"},
		{name: "watchlist cancellation", candidates: validCandidates, reader: &candidateWatchlistReaderStub{err: context.Canceled}, writer: &candidateEventLinkWriterStub{}, stdout: &bytes.Buffer{}, contains: "retrieve watchlist for event linking"},
		{name: "persistence", candidates: validCandidates, reader: validReader, writer: &candidateEventLinkWriterStub{err: wantErr}, stdout: &bytes.Buffer{}, contains: "persist classified watchlist event links"},
		{name: "writer", candidates: validCandidates, reader: validReader, writer: &candidateEventLinkWriterStub{links: []watchlist.StoredEventLink{storedEventLinkFixture()}}, stdout: errorWriter{err: wantErr}, contains: "encode linked watchlist event candidates"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runLinkWatchlistEvents(
				t.Context(), test.candidates, test.reader, test.writer, test.stdout,
				linkWatchlistEventsCommand{watchlistID: validReader.stored.ID, actor: "classifier"},
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want %q context", err, test.contains)
			}
			if test.name != "writer" && !strings.Contains(err.Error(), "link watchlist event candidates") {
				t.Fatalf("error = %v, want command context", err)
			}
			if test.name == "watchlist cancellation" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error = %v, want context.Canceled", err)
				}
			} else if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want wrapped %v", err, wantErr)
			}
		})
	}
}

func validLinkWatchlistEventsArguments() []string {
	return []string{
		"link-watchlist-events",
		"--id", "00000000-0000-0000-0000-000000000001",
		"--from", "2026-08-01T00:00:00Z",
		"--to", "2026-08-02T00:00:00Z",
		"--limit", "10",
		"--actor", "classifier",
	}
}

type linkCandidateReaderStub struct {
	events      []calendar.StoredEvent
	err         error
	calls       int
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (reader *linkCandidateReaderStub) WatchlistEventCandidates(
	_ context.Context,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]calendar.StoredEvent, error) {
	reader.calls++
	reader.windowStart, reader.windowEnd, reader.limit = windowStart, windowEnd, limit
	return reader.events, reader.err
}

type candidateWatchlistReaderStub struct {
	stored watchlist.StoredWatchlist
	err    error
	calls  int
	id     string
}

func (reader *candidateWatchlistReaderStub) Watchlist(
	_ context.Context,
	id string,
) (watchlist.StoredWatchlist, error) {
	reader.calls++
	reader.id = id
	return reader.stored, reader.err
}

type candidateEventLinkWriterStub struct {
	links           []watchlist.StoredEventLink
	err             error
	calls           int
	watchlistID     string
	classifications []watchlist.EventRelevance
	actor           string
}

type candidateDispatchRepository struct {
	watchlistCommandRepository
	reader candidateWatchlistReaderStub
	writer candidateEventLinkWriterStub
}

func (repository *candidateDispatchRepository) Watchlist(
	ctx context.Context,
	id string,
) (watchlist.StoredWatchlist, error) {
	return repository.reader.Watchlist(ctx, id)
}

func (repository *candidateDispatchRepository) CreateEventLinks(
	ctx context.Context,
	watchlistID string,
	classifications []watchlist.EventRelevance,
	actor string,
) ([]watchlist.StoredEventLink, error) {
	return repository.writer.CreateEventLinks(ctx, watchlistID, classifications, actor)
}

func (writer *candidateEventLinkWriterStub) CreateEventLinks(
	_ context.Context,
	watchlistID string,
	classifications []watchlist.EventRelevance,
	actor string,
) ([]watchlist.StoredEventLink, error) {
	writer.calls++
	writer.watchlistID, writer.classifications, writer.actor = watchlistID, classifications, actor
	return writer.links, writer.err
}
