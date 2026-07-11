package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

func TestGenerateDailyBriefResolvesCanonicalCitations(t *testing.T) {
	input := dailyBriefGenerationInput()
	draft := dailyBriefDraft{sections: []dailyBriefSectionDraft{
		{
			heading: "Growth is slowing",
			content: "Recent reporting points to softer activity.",
			citations: []dailyBriefCitationReference{
				{kind: dailyBriefCitationSourceRecord, id: "record-news"},
				{kind: dailyBriefCitationUpcomingEvent, id: "event-gdp"},
			},
		},
		{
			heading: "What to watch",
			content: "The GDP release is the next scheduled catalyst.",
			citations: []dailyBriefCitationReference{
				{kind: dailyBriefCitationUpcomingEvent, id: "event-gdp"},
			},
		},
	}}
	generator := &dailyBriefGeneratorStub{draft: draft}

	got, err := generateDailyBrief(t.Context(), generator, input)
	if err != nil {
		t.Fatalf("generateDailyBrief() error = %v", err)
	}

	want := dailyBrief{
		region: calendar.RegionUnitedStates,
		sections: []dailyBriefSection{
			{
				heading: "Growth is slowing",
				content: "Recent reporting points to softer activity.",
				citations: []dailyBriefCitation{
					{
						kind:   dailyBriefCitationSourceRecord,
						id:     "record-news",
						source: "example-news",
						url:    "https://example.com/news/news",
					},
					{
						kind:   dailyBriefCitationUpcomingEvent,
						id:     "event-gdp",
						source: "official-calendar",
						url:    "https://example.com/calendar/gdp",
					},
				},
			},
			{
				heading: "What to watch",
				content: "The GDP release is the next scheduled catalyst.",
				citations: []dailyBriefCitation{
					{
						kind:   dailyBriefCitationUpcomingEvent,
						id:     "event-gdp",
						source: "official-calendar",
						url:    "https://example.com/calendar/gdp",
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("generateDailyBrief() = %#v, want %#v", got, want)
	}
	if generator.calls != 1 || !reflect.DeepEqual(generator.input, input) {
		t.Errorf("generator call = (%d, %#v), want (1, %#v)", generator.calls, generator.input, input)
	}
}

func TestGenerateDailyBriefRejectsInvalidDraft(t *testing.T) {
	validSection := dailyBriefSectionDraft{
		heading: "What matters",
		content: "A cited development.",
		citations: []dailyBriefCitationReference{
			{kind: dailyBriefCitationSourceRecord, id: "record-news"},
		},
	}
	tests := []struct {
		name     string
		draft    dailyBriefDraft
		contains string
	}{
		{name: "empty output", draft: dailyBriefDraft{}, contains: "at least one section is required"},
		{name: "blank heading", draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{
			withDailyBriefSection(validSection, func(section *dailyBriefSectionDraft) { section.heading = " \t" }),
		}}, contains: "section 0 heading is required"},
		{name: "blank content", draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{
			withDailyBriefSection(validSection, func(section *dailyBriefSectionDraft) { section.content = "\n" }),
		}}, contains: "section 0 content is required"},
		{name: "uncited section", draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{
			withDailyBriefSection(validSection, func(section *dailyBriefSectionDraft) { section.citations = nil }),
		}}, contains: "section 0 must have at least one citation"},
		{name: "unsupported citation", draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{
			withDailyBriefSection(validSection, func(section *dailyBriefSectionDraft) {
				section.citations = []dailyBriefCitationReference{{kind: "market_data", id: "price-1"}}
			}),
		}}, contains: "unsupported citation kind"},
		{name: "unknown source record", draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{
			withDailyBriefSection(validSection, func(section *dailyBriefSectionDraft) {
				section.citations = []dailyBriefCitationReference{{kind: dailyBriefCitationSourceRecord, id: "missing"}}
			}),
		}}, contains: "source record \"missing\" is not present"},
		{name: "unknown upcoming event", draft: dailyBriefDraft{sections: []dailyBriefSectionDraft{
			withDailyBriefSection(validSection, func(section *dailyBriefSectionDraft) {
				section.citations = []dailyBriefCitationReference{{kind: dailyBriefCitationUpcomingEvent, id: "missing"}}
			}),
		}}, contains: "upcoming event \"missing\" is not present"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := generateDailyBrief(
				t.Context(),
				&dailyBriefGeneratorStub{draft: test.draft},
				dailyBriefGenerationInput(),
			)
			if err == nil || !strings.Contains(err.Error(), "validate generated daily brief") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("generateDailyBrief() error = %v, want validation error containing %q", err, test.contains)
			}
			if !reflect.DeepEqual(got, dailyBrief{}) {
				t.Errorf("generateDailyBrief() = %#v, want zero value", got)
			}
		})
	}
}

func TestGenerateDailyBriefPreservesProviderFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "provider failure", err: errors.New("provider unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := generateDailyBrief(
				t.Context(),
				&dailyBriefGeneratorStub{err: test.err},
				dailyBriefGenerationInput(),
			)
			if err == nil || !errors.Is(err, test.err) || !strings.Contains(err.Error(), "generate daily brief with provider") {
				t.Fatalf("generateDailyBrief() error = %v, want contextual %v", err, test.err)
			}
			if !reflect.DeepEqual(got, dailyBrief{}) {
				t.Errorf("generateDailyBrief() = %#v, want zero value", got)
			}
		})
	}
}

type dailyBriefGeneratorStub struct {
	draft dailyBriefDraft
	err   error
	calls int
	input dailyBriefInput
}

func (generator *dailyBriefGeneratorStub) Generate(
	_ context.Context,
	input dailyBriefInput,
) (dailyBriefDraft, error) {
	generator.calls++
	generator.input = input
	return generator.draft, generator.err
}

func dailyBriefGenerationInput() dailyBriefInput {
	publicationStart := time.Date(2026, time.July, 11, 6, 0, 0, 0, time.UTC)
	eventStart := publicationStart.Add(24 * time.Hour)
	return dailyBriefInput{
		region:                 calendar.RegionUnitedStates,
		publicationWindowStart: publicationStart,
		publicationWindowEnd:   publicationStart.Add(12 * time.Hour),
		eventWindowStart:       eventStart,
		eventWindowEnd:         eventStart.Add(48 * time.Hour),
		sourceRecords:          []ingestionpostgres.StoredSourceRecord{newStoredSourceRecord("news", publicationStart)},
		upcomingEvents:         []calendarpostgres.StoredEvent{newStoredEvent("gdp", eventStart)},
	}
}

func withDailyBriefSection(
	section dailyBriefSectionDraft,
	update func(*dailyBriefSectionDraft),
) dailyBriefSectionDraft {
	update(&section)
	return section
}
