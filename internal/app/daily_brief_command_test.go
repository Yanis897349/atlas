package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

func TestParseDailyBriefInputQuery(t *testing.T) {
	query, err := parseDailyBriefInputQuery([]string{
		"--upcoming-event-limit", "7",
		"--publication-to", "2026-07-11T12:30:00+02:00",
		"--region", "united_states",
		"--event-from", "2026-07-12T08:00:00-04:00",
		"--source-record-limit", "12",
		"--publication-from", "2026-07-11T08:00:00Z",
		"--event-to", "2026-07-13T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("parseDailyBriefInputQuery() error = %v", err)
	}

	if query.region != calendar.RegionUnitedStates || query.sourceRecordLimit != 12 || query.upcomingEventLimit != 7 {
		t.Errorf("query classification = (%q, %d, %d), want (%q, 12, 7)", query.region, query.sourceRecordLimit, query.upcomingEventLimit, calendar.RegionUnitedStates)
	}
	wantTimes := []time.Time{
		time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 11, 10, 30, 0, 0, time.UTC),
		time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC),
	}
	gotTimes := []time.Time{
		query.publicationWindowStart,
		query.publicationWindowEnd,
		query.eventWindowStart,
		query.eventWindowEnd,
	}
	for index := range wantTimes {
		if !gotTimes[index].Equal(wantTimes[index]) {
			t.Errorf("query time[%d] = %v, want %v", index, gotTimes[index], wantTimes[index])
		}
	}
}

func TestParseDailyBriefInputQueryAcceptsEqualInclusiveWindows(t *testing.T) {
	arguments := validDailyBriefInputArguments()
	arguments = replaceFlag(arguments, "--publication-to", "2026-07-11T08:00:00Z")
	arguments = replaceFlag(arguments, "--event-to", "2026-07-12T08:00:00Z")
	if _, err := parseDailyBriefInputQuery(arguments[1:]); err != nil {
		t.Fatalf("parseDailyBriefInputQuery() error = %v", err)
	}
}

func TestRunRejectsInvalidDailyBriefInputArgumentsBeforeDatabaseSetup(t *testing.T) {
	tests := []struct {
		name     string
		update   func([]string) []string
		contains string
	}{
		{name: "missing region", update: func(valid []string) []string { return withoutFlag(valid, "--region") }, contains: "--region is required"},
		{name: "missing publication from", update: func(valid []string) []string { return withoutFlag(valid, "--publication-from") }, contains: "--publication-from is required"},
		{name: "missing publication to", update: func(valid []string) []string { return withoutFlag(valid, "--publication-to") }, contains: "--publication-to is required"},
		{name: "missing source limit", update: func(valid []string) []string { return withoutFlag(valid, "--source-record-limit") }, contains: "--source-record-limit is required"},
		{name: "missing event from", update: func(valid []string) []string { return withoutFlag(valid, "--event-from") }, contains: "--event-from is required"},
		{name: "missing event to", update: func(valid []string) []string { return withoutFlag(valid, "--event-to") }, contains: "--event-to is required"},
		{name: "missing event limit", update: func(valid []string) []string { return withoutFlag(valid, "--upcoming-event-limit") }, contains: "--upcoming-event-limit is required"},
		{name: "unknown flag", update: func(valid []string) []string { return append(valid, "--format", "yaml") }, contains: "flag provided but not defined"},
		{name: "positional argument", update: func(valid []string) []string { return append(valid, "extra") }, contains: "unexpected positional arguments"},
		{name: "unsupported region", update: func(valid []string) []string { return replaceFlag(valid, "--region", "asia") }, contains: "unsupported region"},
		{name: "malformed publication from", update: func(valid []string) []string { return replaceFlag(valid, "--publication-from", "today") }, contains: "--publication-from must be RFC3339"},
		{name: "malformed publication to", update: func(valid []string) []string { return replaceFlag(valid, "--publication-to", "today") }, contains: "--publication-to must be RFC3339"},
		{name: "reversed publication window", update: func(valid []string) []string { return replaceFlag(valid, "--publication-to", "2026-07-11T07:59:59Z") }, contains: "--publication-to must not be before --publication-from"},
		{name: "zero source limit", update: func(valid []string) []string { return replaceFlag(valid, "--source-record-limit", "0") }, contains: "--source-record-limit must be between 1 and 100"},
		{name: "source limit above maximum", update: func(valid []string) []string { return replaceFlag(valid, "--source-record-limit", "101") }, contains: "--source-record-limit must be between 1 and 100"},
		{name: "malformed event from", update: func(valid []string) []string { return replaceFlag(valid, "--event-from", "today") }, contains: "--event-from must be RFC3339"},
		{name: "malformed event to", update: func(valid []string) []string { return replaceFlag(valid, "--event-to", "today") }, contains: "--event-to must be RFC3339"},
		{name: "reversed event window", update: func(valid []string) []string { return replaceFlag(valid, "--event-to", "2026-07-12T07:59:59Z") }, contains: "--event-to must not be before --event-from"},
		{name: "zero event limit", update: func(valid []string) []string { return replaceFlag(valid, "--upcoming-event-limit", "0") }, contains: "--upcoming-event-limit must be between 1 and 100"},
		{name: "event limit above maximum", update: func(valid []string) []string { return replaceFlag(valid, "--upcoming-event-limit", "101") }, contains: "--upcoming-event-limit must be between 1 and 100"},
	}

	for _, commandName := range []string{"daily-brief-input", "daily-brief"} {
		t.Run(commandName, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					dependencies := Dependencies{Getenv: func(string) string {
						t.Fatal("configuration read for invalid command arguments")
						return ""
					}}
					arguments := test.update(validDailyBriefArguments(commandName))
					err := Run(t.Context(), arguments, dependencies)
					if err == nil || !strings.Contains(err.Error(), test.contains) {
						t.Fatalf("Run() error = %v, want error containing %q", err, test.contains)
					}
				})
			}
		})
	}
}

func TestRunDailyBriefInputWritesContextJSON(t *testing.T) {
	publicationStart := time.Date(2026, time.July, 11, 10, 0, 0, 123000000, time.FixedZone("CEST", 2*60*60))
	publicationEnd := publicationStart.Add(2 * time.Hour)
	eventStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	eventEnd := eventStart.Add(24 * time.Hour)
	record := newStoredSourceRecord("story-1", publicationEnd)
	record.OriginalURL = "https://example.com/news/story-1?market=fx&region=eu"
	event := newStoredEvent("event-1", eventStart)
	event.SourceURL = "https://example.com/calendar/event-1?view=full&lang=en"
	query := dailyBriefInputQuery{
		region:                 calendar.RegionEurozone,
		publicationWindowStart: publicationStart,
		publicationWindowEnd:   publicationEnd,
		sourceRecordLimit:      1,
		eventWindowStart:       eventStart,
		eventWindowEnd:         eventEnd,
		upcomingEventLimit:     1,
	}
	sourceRepository := &dailyBriefSourceRecordsStub{records: []ingestionpostgres.StoredSourceRecord{record}}
	eventRepository := &dailyBriefEventsStub{events: []calendarpostgres.StoredEvent{event}}
	stdout := &bytes.Buffer{}

	if err := runDailyBriefInput(t.Context(), sourceRepository, eventRepository, stdout, query); err != nil {
		t.Fatalf("runDailyBriefInput() error = %v", err)
	}

	want := `{"region":"eurozone","publication_window":{"from":"2026-07-11T08:00:00.123Z","to":"2026-07-11T10:00:00.123Z"},` +
		`"event_window":{"from":"2026-07-12T12:00:00Z","to":"2026-07-13T12:00:00Z"},` +
		`"source_records":[{"id":"record-story-1","source":"example-news","source_item_id":"story-1","original_url":"https://example.com/news/story-1?market=fx&region=eu","title":"Source record story-1","published_at":"2026-07-11T10:00:00.123Z","retrieved_at":"2026-07-11T10:01:00.123Z"}],` +
		`"upcoming_events":[{"id":"event-event-1","source":"official-calendar","external_event_id":"event-1","name":"Economic event event-1","region":"united_states","event_type":"gdp","scheduled_at":"2026-07-12T12:00:00Z","source_url":"https://example.com/calendar/event-1?view=full&lang=en","retrieved_at":"2026-07-12T11:00:00Z"}]}` + "\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunDailyBriefInputWritesEmptyArrays(t *testing.T) {
	stdout := &bytes.Buffer{}
	if err := runDailyBriefInput(
		t.Context(),
		&dailyBriefSourceRecordsStub{},
		&dailyBriefEventsStub{},
		stdout,
		validDailyBriefInputQuery(),
	); err != nil {
		t.Fatalf("runDailyBriefInput() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"source_records":[],"upcoming_events":[]`) {
		t.Errorf("stdout = %q, want empty JSON arrays", stdout.String())
	}
}

func TestRunDailyBriefInputPreservesFailures(t *testing.T) {
	tests := []struct {
		name    string
		sources recentSourceRecordsRepository
		events  dailyBriefEventsRepository
		err     error
		context string
	}{
		{name: "source repository", sources: &dailyBriefSourceRecordsStub{err: errors.New("source unavailable")}, events: panicDailyBriefEvents{}, err: errors.New("source unavailable"), context: "retrieve daily brief source records"},
		{name: "event cancellation", sources: &dailyBriefSourceRecordsStub{}, events: &dailyBriefEventsStub{err: context.Canceled}, err: context.Canceled, context: "retrieve daily brief upcoming events"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runDailyBriefInput(t.Context(), test.sources, test.events, &bytes.Buffer{}, validDailyBriefInputQuery())
			if err == nil || !strings.Contains(err.Error(), "assemble daily brief input") ||
				!strings.Contains(err.Error(), test.context) {
				t.Fatalf("runDailyBriefInput() error = %v, want contextual failure", err)
			}
			if test.err == context.Canceled && !errors.Is(err, context.Canceled) {
				t.Fatalf("runDailyBriefInput() error = %v, want cancellation preserved", err)
			}
		})
	}
}

func TestRunDailyBriefInputReportsWriterFailure(t *testing.T) {
	writeErr := errors.New("output unavailable")
	err := runDailyBriefInput(
		t.Context(),
		&dailyBriefSourceRecordsStub{},
		&dailyBriefEventsStub{},
		errorWriter{err: writeErr},
		validDailyBriefInputQuery(),
	)
	if err == nil || !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "encode daily brief input") {
		t.Fatalf("runDailyBriefInput() error = %v, want contextual writer failure", err)
	}
}

func validDailyBriefInputArguments() []string {
	return validDailyBriefArguments("daily-brief-input")
}

func validDailyBriefArguments(commandName string) []string {
	return []string{
		commandName,
		"--region", "eurozone",
		"--publication-from", "2026-07-11T08:00:00Z",
		"--publication-to", "2026-07-11T12:00:00Z",
		"--source-record-limit", "10",
		"--event-from", "2026-07-12T08:00:00Z",
		"--event-to", "2026-07-13T08:00:00Z",
		"--upcoming-event-limit", "10",
	}
}
