package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
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
	persistence := &dailyBriefPersistenceStub{}

	err := runDailyBrief(
		t.Context(),
		&dailyBriefSourceRecordsStub{records: input.sourceRecords},
		&dailyBriefEventsStub{events: input.upcomingEvents},
		generator,
		persistence,
		stdout,
		validDailyBriefInputQuery(),
	)
	if err != nil {
		t.Fatalf("runDailyBrief() error = %v", err)
	}

	var output dailyBriefOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output.ID != "00000000-0000-0000-0000-000000000001" || output.Region != "eurozone" ||
		output.Provider != "openai" || output.Model != "test-model" || len(output.Sections) != 1 {
		t.Errorf("output = %#v, want stored Eurozone OpenAI brief", output)
	}
	if !strings.Contains(stdout.String(), "<now>") {
		t.Errorf("stdout = %q, want unescaped HTML characters", stdout.String())
	}
	if generator.calls != 1 || len(generator.input.sourceRecords) != 1 || len(generator.input.upcomingEvents) != 1 {
		t.Errorf("generator input = %#v after %d calls", generator.input, generator.calls)
	}
	if persistence.calls != 1 || persistence.actor != dailyBriefGenerationActor {
		t.Errorf("persistence call = (%d, %q), want one generation-actor call", persistence.calls, persistence.actor)
	}
}

func TestRunDailyBriefPreservesFailures(t *testing.T) {
	input := dailyBriefGenerationInput()
	tests := []struct {
		name      string
		sources   recentSourceRecordsRepository
		events    dailyBriefEventsRepository
		generator dailyBriefGenerator
		briefs    dailyBriefPersistence
		wantErr   error
		contains  string
		persisted int
	}{
		{
			name:    "retrieval",
			sources: &dailyBriefSourceRecordsStub{err: errors.New("source unavailable")},
			events:  panicDailyBriefEvents{}, generator: &dailyBriefGeneratorStub{}, briefs: &dailyBriefPersistenceStub{},
			contains: "assemble daily brief input: retrieve daily brief source records",
		},
		{
			name:      "provider cancellation",
			sources:   &dailyBriefSourceRecordsStub{records: input.sourceRecords},
			events:    &dailyBriefEventsStub{events: input.upcomingEvents},
			generator: &dailyBriefGeneratorStub{err: context.Canceled},
			briefs:    &dailyBriefPersistenceStub{},
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
			briefs:   &dailyBriefPersistenceStub{},
			contains: "generate daily brief: validate generated daily brief",
		},
		{
			name:      "persistence",
			sources:   &dailyBriefSourceRecordsStub{records: input.sourceRecords},
			events:    &dailyBriefEventsStub{events: input.upcomingEvents},
			generator: validDailyBriefGenerator(),
			briefs:    &dailyBriefPersistenceStub{err: context.Canceled},
			wantErr:   context.Canceled,
			contains:  "persist daily brief",
			persisted: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runDailyBrief(
				t.Context(), test.sources, test.events, test.generator, test.briefs, &bytes.Buffer{}, validDailyBriefInputQuery(),
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("runDailyBrief() error = %v, want error containing %q", err, test.contains)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("runDailyBrief() error = %v, want wrapped %v", err, test.wantErr)
			}
			if persistence, ok := test.briefs.(*dailyBriefPersistenceStub); ok && persistence.calls != test.persisted {
				t.Errorf("persistence calls = %d, want %d", persistence.calls, test.persisted)
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
		&dailyBriefPersistenceStub{},
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
		&dailyBriefPersistenceStub{},
		errorWriter{err: writeErr},
		validDailyBriefInputQuery(),
	)
	if err == nil || !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "encode daily brief") {
		t.Fatalf("runDailyBrief() error = %v, want contextual writer failure", err)
	}
}

func validDailyBriefGenerator() dailyBriefGenerator {
	return &dailyBriefGeneratorStub{draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{{
		heading: "Section",
		content: "Cited content.",
		citations: []dailyBriefCitationReference{{
			kind: dailyBriefCitationSourceRecord,
			id:   "record-news",
		}},
	}}}}
}

type dailyBriefPersistenceStub struct {
	stored storedDailyBrief
	err    error
	calls  int
	brief  dailyBrief
	actor  string
}

func (repository *dailyBriefPersistenceStub) PersistDailyBrief(
	_ context.Context,
	brief dailyBrief,
	actor string,
) (storedDailyBrief, error) {
	repository.calls++
	repository.brief = brief
	repository.actor = actor
	if repository.err != nil {
		return storedDailyBrief{}, repository.err
	}
	if repository.stored.ID != "" {
		return repository.stored, nil
	}
	createdAt := time.Date(2026, time.July, 11, 20, 0, 0, 0, time.UTC)
	return storedDailyBrief{
		ID:         "00000000-0000-0000-0000-000000000001",
		dailyBrief: brief,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
		CreatedBy:  actor,
		UpdatedBy:  actor,
	}, nil
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
