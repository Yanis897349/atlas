package dailybrief

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Generate invokes a provider and resolves every citation to canonical input metadata.
func Generate(
	ctx context.Context,
	generator Generator,
	input Input,
) (Brief, error) {
	generation, err := generator.Generate(ctx, input)
	if err != nil {
		return Brief{}, fmt.Errorf("generate daily brief with provider: %w", err)
	}

	brief, err := resolve(input, generation)
	if err != nil {
		return Brief{}, fmt.Errorf("validate generated daily brief: %w", err)
	}
	return brief, nil
}

func resolve(input Input, generation Generation) (Brief, error) {
	draft := generation.Draft
	if strings.TrimSpace(generation.Provider) == "" {
		return Brief{}, errors.New("provider is required")
	}
	if strings.TrimSpace(generation.Model) == "" {
		return Brief{}, errors.New("model is required")
	}
	if len(draft.Sections) == 0 {
		return Brief{}, errors.New("at least one section is required")
	}

	brief := Brief{
		Region:                 input.Region,
		PublicationWindowStart: input.PublicationWindowStart,
		PublicationWindowEnd:   input.PublicationWindowEnd,
		EventWindowStart:       input.EventWindowStart,
		EventWindowEnd:         input.EventWindowEnd,
		Provider:               strings.TrimSpace(generation.Provider),
		Model:                  strings.TrimSpace(generation.Model),
		Sections:               make([]Section, 0, len(draft.Sections)),
	}
	for sectionIndex, draftSection := range draft.Sections {
		if strings.TrimSpace(draftSection.Heading) == "" {
			return Brief{}, fmt.Errorf("section %d heading is required", sectionIndex)
		}
		if strings.TrimSpace(draftSection.Content) == "" {
			return Brief{}, fmt.Errorf("section %d content is required", sectionIndex)
		}
		if len(draftSection.Citations) == 0 {
			return Brief{}, fmt.Errorf("section %d must have at least one citation", sectionIndex)
		}

		section := Section{
			Heading:   draftSection.Heading,
			Content:   draftSection.Content,
			Citations: make([]Citation, 0, len(draftSection.Citations)),
		}
		for citationIndex, reference := range draftSection.Citations {
			citation, err := resolveCitation(input, reference)
			if err != nil {
				return Brief{}, fmt.Errorf(
					"section %d citation %d: %w",
					sectionIndex,
					citationIndex,
					err,
				)
			}
			section.Citations = append(section.Citations, citation)
		}
		brief.Sections = append(brief.Sections, section)
	}

	return brief, nil
}

func resolveCitation(input Input, reference CitationReference) (Citation, error) {
	switch reference.Kind {
	case CitationSourceRecord:
		for _, record := range input.SourceRecords {
			if record.ID == reference.ID {
				return Citation{
					Kind:   reference.Kind,
					ID:     record.ID,
					Source: record.Source,
					URL:    record.OriginalURL,
				}, nil
			}
		}
		return Citation{}, fmt.Errorf("source record %q is not present in the daily brief input", reference.ID)
	case CitationUpcomingEvent:
		for _, event := range input.UpcomingEvents {
			if event.ID == reference.ID {
				return Citation{
					Kind:   reference.Kind,
					ID:     event.ID,
					Source: event.Source,
					URL:    event.SourceURL,
				}, nil
			}
		}
		return Citation{}, fmt.Errorf("upcoming event %q is not present in the daily brief input", reference.ID)
	default:
		return Citation{}, fmt.Errorf("unsupported citation kind %q", reference.Kind)
	}
}
