package dailybrief

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestGenerateResolvesCanonicalCitations(t *testing.T) {
	input := generationInput()
	draft := Draft{Sections: []SectionDraft{
		{Heading: "Growth is slowing", Content: "Recent reporting points to softer activity.", Citations: []CitationReference{
			{Kind: CitationSourceRecord, ID: "record-news"}, {Kind: CitationUpcomingEvent, ID: "event-gdp"},
		}},
		{Heading: "What to watch", Content: "The GDP release is next.", Citations: []CitationReference{{Kind: CitationUpcomingEvent, ID: "event-gdp"}}},
	}}
	generator := &generatorStub{draft: draft}

	got, err := Generate(t.Context(), generator, input)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := Brief{
		Region: input.Region, PublicationWindowStart: input.PublicationWindowStart, PublicationWindowEnd: input.PublicationWindowEnd,
		EventWindowStart: input.EventWindowStart, EventWindowEnd: input.EventWindowEnd, Provider: "openai", Model: "test-model",
		Sections: []Section{
			{Heading: "Growth is slowing", Content: "Recent reporting points to softer activity.", Citations: []Citation{
				{Kind: CitationSourceRecord, ID: "record-news", Source: "example-news", URL: "https://example.com/news/news"},
				{Kind: CitationUpcomingEvent, ID: "event-gdp", Source: "official-calendar", URL: "https://example.com/calendar/gdp"},
			}},
			{Heading: "What to watch", Content: "The GDP release is next.", Citations: []Citation{
				{Kind: CitationUpcomingEvent, ID: "event-gdp", Source: "official-calendar", URL: "https://example.com/calendar/gdp"},
			}},
		},
	}
	if !reflect.DeepEqual(got, want) || generator.calls != 1 || !reflect.DeepEqual(generator.input, input) {
		t.Errorf("Generate() = %#v, want %#v", got, want)
	}
}

func TestGenerateRejectsInvalidDraft(t *testing.T) {
	valid := SectionDraft{Heading: "What matters", Content: "A cited development.", Citations: []CitationReference{{Kind: CitationSourceRecord, ID: "record-news"}}}
	tests := []struct {
		name     string
		draft    Draft
		contains string
	}{
		{name: "empty output", draft: Draft{}, contains: "at least one section is required"},
		{name: "blank heading", draft: Draft{Sections: []SectionDraft{withSection(valid, func(section *SectionDraft) { section.Heading = " " })}}, contains: "heading is required"},
		{name: "blank content", draft: Draft{Sections: []SectionDraft{withSection(valid, func(section *SectionDraft) { section.Content = "\n" })}}, contains: "content is required"},
		{name: "uncited section", draft: Draft{Sections: []SectionDraft{withSection(valid, func(section *SectionDraft) { section.Citations = nil })}}, contains: "must have at least one citation"},
		{name: "unsupported citation", draft: Draft{Sections: []SectionDraft{withSection(valid, func(section *SectionDraft) {
			section.Citations = []CitationReference{{Kind: "market_data", ID: "price-1"}}
		})}}, contains: "unsupported citation kind"},
		{name: "unknown source", draft: Draft{Sections: []SectionDraft{withSection(valid, func(section *SectionDraft) {
			section.Citations = []CitationReference{{Kind: CitationSourceRecord, ID: "missing"}}
		})}}, contains: "source record \"missing\" is not present"},
		{name: "unknown event", draft: Draft{Sections: []SectionDraft{withSection(valid, func(section *SectionDraft) {
			section.Citations = []CitationReference{{Kind: CitationUpcomingEvent, ID: "missing"}}
		})}}, contains: "upcoming event \"missing\" is not present"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Generate(t.Context(), &generatorStub{draft: test.draft}, generationInput())
			if err == nil || !strings.Contains(err.Error(), "validate generated daily brief") || !strings.Contains(err.Error(), test.contains) || !reflect.DeepEqual(got, Brief{}) {
				t.Fatalf("Generate() = (%#v, %v), want validation error containing %q", got, err, test.contains)
			}
		})
	}
}

func TestResolveRequiresProviderProvenance(t *testing.T) {
	valid := Draft{Sections: []SectionDraft{{Heading: "What matters", Content: "A cited development.", Citations: []CitationReference{{Kind: CitationSourceRecord, ID: "record-news"}}}}}
	for _, test := range []struct {
		name       string
		generation Generation
		contains   string
	}{
		{name: "missing provider", generation: Generation{Model: "model", Draft: valid}, contains: "provider is required"},
		{name: "missing model", generation: Generation{Provider: "provider", Draft: valid}, contains: "model is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolve(generationInput(), test.generation); err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("resolve() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestGeneratePreservesProviderFailures(t *testing.T) {
	for _, wantErr := range []error{errors.New("provider unavailable"), context.Canceled} {
		got, err := Generate(t.Context(), &generatorStub{err: wantErr}, generationInput())
		if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "generate daily brief with provider") || !reflect.DeepEqual(got, Brief{}) {
			t.Fatalf("Generate() = (%#v, %v), want contextual %v", got, err, wantErr)
		}
	}
}

type generatorStub struct {
	provider string
	model    string
	draft    Draft
	err      error
	calls    int
	input    Input
}

func (generator *generatorStub) Generate(_ context.Context, input Input) (Generation, error) {
	generator.calls++
	generator.input = input
	provider, model := generator.provider, generator.model
	if provider == "" {
		provider = "openai"
	}
	if model == "" {
		model = "test-model"
	}
	return Generation{Provider: provider, Model: model, Draft: generator.draft}, generator.err
}

func generationInput() Input {
	publicationStart := time.Date(2026, time.July, 11, 6, 0, 0, 0, time.UTC)
	eventStart := publicationStart.Add(24 * time.Hour)
	return Input{
		Region: calendar.RegionUnitedStates, PublicationWindowStart: publicationStart, PublicationWindowEnd: publicationStart.Add(12 * time.Hour),
		EventWindowStart: eventStart, EventWindowEnd: eventStart.Add(48 * time.Hour),
		SourceRecords:  []ingestion.StoredSourceRecord{newStoredSourceRecord("news", publicationStart)},
		UpcomingEvents: []calendar.StoredEvent{newStoredEvent("gdp", eventStart)},
	}
}

func withSection(section SectionDraft, update func(*SectionDraft)) SectionDraft {
	update(&section)
	return section
}
