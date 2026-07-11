package app

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

func TestParseUpcomingEventsQuery(t *testing.T) {
	query, err := parseUpcomingEventsQuery([]string{
		"--limit", "25",
		"--to", "2026-08-02T10:30:00+02:00",
		"--region", "united_states",
		"--from", "2026-08-01T08:00:00Z",
	})
	if err != nil {
		t.Fatalf("parseUpcomingEventsQuery() error = %v", err)
	}

	wantStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.August, 2, 8, 30, 0, 0, time.UTC)
	if query.region != calendar.RegionUnitedStates || query.limit != 25 {
		t.Errorf("query classification = (%q, %d), want (%q, 25)", query.region, query.limit, calendar.RegionUnitedStates)
	}
	if !query.windowStart.Equal(wantStart) || !query.windowEnd.Equal(wantEnd) {
		t.Errorf("query window = (%v, %v), want (%v, %v)", query.windowStart, query.windowEnd, wantStart, wantEnd)
	}
}

func TestRunRejectsInvalidUpcomingEventsArgumentsBeforeDatabaseSetup(t *testing.T) {
	valid := []string{
		"upcoming-events",
		"--region", "eurozone",
		"--from", "2026-08-01T08:00:00Z",
		"--to", "2026-08-02T08:00:00Z",
		"--limit", "10",
	}
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing region", arguments: withoutFlag(valid, "--region"), contains: "--region is required"},
		{name: "missing from", arguments: withoutFlag(valid, "--from"), contains: "--from is required"},
		{name: "missing to", arguments: withoutFlag(valid, "--to"), contains: "--to is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "unknown flag", arguments: append(append([]string{}, valid...), "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(append([]string{}, valid...), "extra"), contains: "unexpected positional arguments"},
		{name: "unsupported region", arguments: replaceFlag(valid, "--region", "asia"), contains: "unsupported region"},
		{name: "malformed from", arguments: replaceFlag(valid, "--from", "tomorrow"), contains: "--from must be RFC3339"},
		{name: "malformed to", arguments: replaceFlag(valid, "--to", "tomorrow"), contains: "--to must be RFC3339"},
		{name: "reversed window", arguments: replaceFlag(valid, "--to", "2026-07-31T08:00:00Z"), contains: "--to must not be before --from"},
		{name: "zero limit", arguments: replaceFlag(valid, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "limit above maximum", arguments: replaceFlag(valid, "--limit", "101"), contains: "--limit must be between 1 and 100"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dependencies := Dependencies{Getenv: func(string) string {
				t.Fatal("database configuration read for invalid command arguments")
				return ""
			}}
			err := Run(t.Context(), test.arguments, dependencies)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run() error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestRunUpcomingEventsWritesDomainJSON(t *testing.T) {
	scheduledAt := time.Date(2026, time.August, 1, 10, 30, 0, 123000000, time.FixedZone("CEST", 2*60*60))
	retrievedAt := time.Date(2026, time.July, 11, 9, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	repository := &upcomingEventsStub{events: []calendar.StoredEvent{
		{
			ID: "first-id",
			Event: calendar.Event{
				Source:          "official-source",
				ExternalEventID: "event-1",
				Name:            "First event",
				Region:          calendar.RegionEurozone,
				Type:            calendar.EventTypeGDP,
				ScheduledAt:     scheduledAt,
				SourceURL:       "https://example.com/calendar?event=1&view=full",
				RetrievedAt:     retrievedAt,
			},
			CreatedBy: "not-exposed",
		},
		{
			ID: "second-id",
			Event: calendar.Event{
				Source:          "official-source",
				ExternalEventID: "event-2",
				Name:            "Second event",
				Region:          calendar.RegionEurozone,
				Type:            calendar.EventTypeInterestRateDecision,
				ScheduledAt:     scheduledAt,
				SourceURL:       "https://example.com/calendar/event-2",
				RetrievedAt:     retrievedAt,
			},
		},
	}}
	query := upcomingEventsQuery{
		region:      calendar.RegionEurozone,
		windowStart: scheduledAt.Add(-time.Hour),
		windowEnd:   scheduledAt.Add(time.Hour),
		limit:       2,
	}
	stdout := &bytes.Buffer{}

	if err := runUpcomingEvents(t.Context(), repository, stdout, query); err != nil {
		t.Fatalf("runUpcomingEvents() error = %v", err)
	}

	if repository.calls != 1 || !reflect.DeepEqual(repository.query, query) {
		t.Errorf("repository call = (%d, %#v), want (1, %#v)", repository.calls, repository.query, query)
	}
	want := "[" +
		`{"id":"first-id","source":"official-source","external_event_id":"event-1","name":"First event","region":"eurozone","event_type":"gdp","scheduled_at":"2026-08-01T08:30:00.123Z","source_url":"https://example.com/calendar?event=1&view=full","retrieved_at":"2026-07-11T07:00:00Z"},` +
		`{"id":"second-id","source":"official-source","external_event_id":"event-2","name":"Second event","region":"eurozone","event_type":"interest_rate_decision","scheduled_at":"2026-08-01T08:30:00.123Z","source_url":"https://example.com/calendar/event-2","retrieved_at":"2026-07-11T07:00:00Z"}` +
		"]\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunUpcomingEventsWritesEmptyArray(t *testing.T) {
	stdout := &bytes.Buffer{}
	if err := runUpcomingEvents(t.Context(), &upcomingEventsStub{}, stdout, upcomingEventsQuery{}); err != nil {
		t.Fatalf("runUpcomingEvents() error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("stdout = %q, want empty JSON array", stdout.String())
	}
}

func TestRunUpcomingEventsPreservesRepositoryFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "database failure", err: errors.New("database unavailable")},
		{name: "cancellation", err: context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runUpcomingEvents(t.Context(), &upcomingEventsStub{err: test.err}, &bytes.Buffer{}, upcomingEventsQuery{})
			if err == nil || !errors.Is(err, test.err) || !strings.Contains(err.Error(), "list upcoming economic events") {
				t.Fatalf("runUpcomingEvents() error = %v, want contextual %v", err, test.err)
			}
		})
	}
}

func TestRunUpcomingEventsReportsWriterFailure(t *testing.T) {
	writeErr := errors.New("output unavailable")
	err := runUpcomingEvents(t.Context(), &upcomingEventsStub{}, errorWriter{err: writeErr}, upcomingEventsQuery{})
	if err == nil || !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "encode upcoming economic events") {
		t.Fatalf("runUpcomingEvents() error = %v, want contextual writer failure", err)
	}
}

type upcomingEventsStub struct {
	events []calendar.StoredEvent
	err    error
	calls  int
	query  upcomingEventsQuery
}

func (repository *upcomingEventsStub) UpcomingEvents(
	_ context.Context,
	region calendar.Region,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]calendar.StoredEvent, error) {
	repository.calls++
	repository.query = upcomingEventsQuery{region: region, windowStart: windowStart, windowEnd: windowEnd, limit: limit}
	return repository.events, repository.err
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func withoutFlag(arguments []string, name string) []string {
	result := append([]string{}, arguments...)
	for index := range result {
		if result[index] == name {
			return append(result[:index], result[index+2:]...)
		}
	}
	return result
}

func replaceFlag(arguments []string, name, value string) []string {
	result := append([]string{}, arguments...)
	for index := range result {
		if result[index] == name {
			result[index+1] = value
			return result
		}
	}
	return result
}
