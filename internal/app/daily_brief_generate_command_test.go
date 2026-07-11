package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunDailyBriefWritesCanonicalCitedJSON(t *testing.T) {
	input := dailyBriefGenerationInput()
	generator := &dailyBriefGeneratorStub{draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{
		{
			heading: "What to watch & why",
			content: "The release matters <now>.",
			citations: []dailyBriefCitationReference{
				{kind: dailyBriefCitationUpcomingEvent, id: "event-gdp"},
				{kind: dailyBriefCitationSourceRecord, id: "record-news"},
			},
		},
	}}}
	stdout := &bytes.Buffer{}

	err := runDailyBrief(
		t.Context(),
		&dailyBriefSourceRecordsStub{records: input.sourceRecords},
		&dailyBriefEventsStub{events: input.upcomingEvents},
		generator,
		stdout,
		validDailyBriefInputQuery(),
	)
	if err != nil {
		t.Fatalf("runDailyBrief() error = %v", err)
	}

	want := `{"region":"eurozone","sections":[{"heading":"What to watch & why","content":"The release matters <now>.","citations":[` +
		`{"kind":"upcoming_event","id":"event-gdp","source":"official-calendar","url":"https://example.com/calendar/gdp"},` +
		`{"kind":"source_record","id":"record-news","source":"example-news","url":"https://example.com/news/news"}]}]}` + "\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if generator.calls != 1 || len(generator.input.sourceRecords) != 1 || len(generator.input.upcomingEvents) != 1 {
		t.Errorf("generator input = %#v after %d calls", generator.input, generator.calls)
	}
}

func TestRunDailyBriefPreservesFailures(t *testing.T) {
	input := dailyBriefGenerationInput()
	tests := []struct {
		name      string
		sources   recentSourceRecordsRepository
		events    dailyBriefEventsRepository
		generator dailyBriefGenerator
		wantErr   error
		contains  string
	}{
		{
			name:    "retrieval",
			sources: &dailyBriefSourceRecordsStub{err: errors.New("source unavailable")},
			events:  panicDailyBriefEvents{}, generator: &dailyBriefGeneratorStub{},
			contains: "assemble daily brief input: retrieve daily brief source records",
		},
		{
			name:      "provider cancellation",
			sources:   &dailyBriefSourceRecordsStub{records: input.sourceRecords},
			events:    &dailyBriefEventsStub{events: input.upcomingEvents},
			generator: &dailyBriefGeneratorStub{err: context.Canceled},
			wantErr:   context.Canceled, contains: "generate daily brief: generate daily brief with provider",
		},
		{
			name:    "citation validation",
			sources: &dailyBriefSourceRecordsStub{records: input.sourceRecords},
			events:  &dailyBriefEventsStub{events: input.upcomingEvents},
			generator: &dailyBriefGeneratorStub{draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{{
				heading: "Invalid", content: "Unknown source.", citations: []dailyBriefCitationReference{{
					kind: dailyBriefCitationSourceRecord, id: "missing",
				}},
			}}}},
			contains: "generate daily brief: validate generated daily brief",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runDailyBrief(
				t.Context(), test.sources, test.events, test.generator, &bytes.Buffer{}, validDailyBriefInputQuery(),
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("runDailyBrief() error = %v, want error containing %q", err, test.contains)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("runDailyBrief() error = %v, want wrapped %v", err, test.wantErr)
			}
		})
	}
}

func TestRunDailyBriefPassesEmptyRetrievedInputToGenerator(t *testing.T) {
	providerErr := errors.New("generation unavailable")
	generator := &dailyBriefGeneratorStub{err: providerErr}
	err := runDailyBrief(
		t.Context(),
		&dailyBriefSourceRecordsStub{},
		&dailyBriefEventsStub{},
		generator,
		&bytes.Buffer{},
		validDailyBriefInputQuery(),
	)
	if err == nil || !errors.Is(err, providerErr) {
		t.Fatalf("runDailyBrief() error = %v, want provider failure", err)
	}
	if generator.calls != 1 || len(generator.input.sourceRecords) != 0 || len(generator.input.upcomingEvents) != 0 {
		t.Errorf("generator input = %#v after %d calls, want empty retrieved slices", generator.input, generator.calls)
	}
}

func TestRunDailyBriefReportsWriterFailure(t *testing.T) {
	input := dailyBriefGenerationInput()
	writeErr := errors.New("output unavailable")
	err := runDailyBrief(
		t.Context(),
		&dailyBriefSourceRecordsStub{records: input.sourceRecords},
		&dailyBriefEventsStub{events: input.upcomingEvents},
		&dailyBriefGeneratorStub{draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{{
			heading: "Section", content: "Cited content.", citations: []dailyBriefCitationReference{{
				kind: dailyBriefCitationSourceRecord, id: "record-news",
			}},
		}}}},
		errorWriter{err: writeErr},
		validDailyBriefInputQuery(),
	)
	if err == nil || !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "encode daily brief") {
		t.Fatalf("runDailyBrief() error = %v, want contextual writer failure", err)
	}
}

func TestRunDailyBriefValidatesProviderConfigurationBeforeConnecting(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		contains string
	}{
		{name: "missing API key", env: map[string]string{"ATLAS_OPENAI_MODEL": "test-model"}, contains: "OpenAI API key is required"},
		{name: "missing model", env: map[string]string{"ATLAS_OPENAI_API_KEY": "secret"}, contains: "OpenAI model is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.env["ATLAS_DATABASE_URL"] = "postgres://atlas:secret@127.0.0.1:1/atlas?sslmode=disable"
			err := Run(t.Context(), validDailyBriefArguments("daily-brief"), Dependencies{
				Getenv: func(name string) string { return test.env[name] },
			})
			if err == nil || !strings.Contains(err.Error(), "configure OpenAI daily brief generator") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(daily-brief) error = %v, want configuration error containing %q", err, test.contains)
			}
		})
	}
}
