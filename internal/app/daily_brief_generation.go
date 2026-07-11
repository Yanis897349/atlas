package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Yanis897349/atlas/internal/calendar"
)

type dailyBriefCitationKind string

const (
	dailyBriefCitationSourceRecord  dailyBriefCitationKind = "source_record"
	dailyBriefCitationUpcomingEvent dailyBriefCitationKind = "upcoming_event"
)

type dailyBriefCitationReference struct {
	kind dailyBriefCitationKind
	id   string
}

type dailyBriefSectionDraft struct {
	heading   string
	content   string
	citations []dailyBriefCitationReference
}

type dailyBriefDraft struct {
	sections []dailyBriefSectionDraft
}

type dailyBriefCitation struct {
	kind   dailyBriefCitationKind
	id     string
	source string
	url    string
}

type dailyBriefSection struct {
	heading   string
	content   string
	citations []dailyBriefCitation
}

type dailyBrief struct {
	region   calendar.Region
	sections []dailyBriefSection
}

type dailyBriefGenerator interface {
	Generate(context.Context, dailyBriefInput) (dailyBriefDraft, error)
}

func generateDailyBrief(
	ctx context.Context,
	generator dailyBriefGenerator,
	input dailyBriefInput,
) (dailyBrief, error) {
	draft, err := generator.Generate(ctx, input)
	if err != nil {
		return dailyBrief{}, fmt.Errorf("generate daily brief with provider: %w", err)
	}

	brief, err := resolveDailyBrief(input, draft)
	if err != nil {
		return dailyBrief{}, fmt.Errorf("validate generated daily brief: %w", err)
	}
	return brief, nil
}

func resolveDailyBrief(input dailyBriefInput, draft dailyBriefDraft) (dailyBrief, error) {
	if len(draft.sections) == 0 {
		return dailyBrief{}, errors.New("at least one section is required")
	}

	brief := dailyBrief{
		region:   input.region,
		sections: make([]dailyBriefSection, 0, len(draft.sections)),
	}
	for sectionIndex, draftSection := range draft.sections {
		if strings.TrimSpace(draftSection.heading) == "" {
			return dailyBrief{}, fmt.Errorf("section %d heading is required", sectionIndex)
		}
		if strings.TrimSpace(draftSection.content) == "" {
			return dailyBrief{}, fmt.Errorf("section %d content is required", sectionIndex)
		}
		if len(draftSection.citations) == 0 {
			return dailyBrief{}, fmt.Errorf("section %d must have at least one citation", sectionIndex)
		}

		section := dailyBriefSection{
			heading:   draftSection.heading,
			content:   draftSection.content,
			citations: make([]dailyBriefCitation, 0, len(draftSection.citations)),
		}
		for citationIndex, reference := range draftSection.citations {
			citation, err := resolveDailyBriefCitation(input, reference)
			if err != nil {
				return dailyBrief{}, fmt.Errorf(
					"section %d citation %d: %w",
					sectionIndex,
					citationIndex,
					err,
				)
			}
			section.citations = append(section.citations, citation)
		}
		brief.sections = append(brief.sections, section)
	}

	return brief, nil
}

func resolveDailyBriefCitation(
	input dailyBriefInput,
	reference dailyBriefCitationReference,
) (dailyBriefCitation, error) {
	switch reference.kind {
	case dailyBriefCitationSourceRecord:
		for _, record := range input.sourceRecords {
			if record.ID == reference.id {
				return dailyBriefCitation{
					kind:   reference.kind,
					id:     record.ID,
					source: record.Source,
					url:    record.OriginalURL,
				}, nil
			}
		}
		return dailyBriefCitation{}, fmt.Errorf("source record %q is not present in the daily brief input", reference.id)
	case dailyBriefCitationUpcomingEvent:
		for _, event := range input.upcomingEvents {
			if event.ID == reference.id {
				return dailyBriefCitation{
					kind:   reference.kind,
					id:     event.ID,
					source: event.Source,
					url:    event.SourceURL,
				}, nil
			}
		}
		return dailyBriefCitation{}, fmt.Errorf("upcoming event %q is not present in the daily brief input", reference.id)
	default:
		return dailyBriefCitation{}, fmt.Errorf("unsupported citation kind %q", reference.kind)
	}
}
