package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
)

type dailyBriefCitationOutput struct {
	Kind   dailyBriefCitationKind `json:"kind"`
	ID     string                 `json:"id"`
	Source string                 `json:"source"`
	URL    string                 `json:"url"`
}

type dailyBriefSectionOutput struct {
	Heading   string                     `json:"heading"`
	Content   string                     `json:"content"`
	Citations []dailyBriefCitationOutput `json:"citations"`
}

type dailyBriefOutput struct {
	Region   calendar.Region           `json:"region"`
	Sections []dailyBriefSectionOutput `json:"sections"`
}

func configureOpenAIDailyBriefGenerator(
	getenv func(string) string,
	dependencies Dependencies,
) (dailyBriefGenerator, error) {
	generator, err := newOpenAIDailyBriefGenerator(openAIDailyBriefGeneratorConfig{
		APIKey:        getenv("ATLAS_OPENAI_API_KEY"),
		Model:         getenv("ATLAS_OPENAI_MODEL"),
		Client:        dependencies.OpenAIHTTPClient,
		Endpoint:      dependencies.OpenAIEndpoint,
		RequestBudget: dependencies.OpenAIRequestBudget,
	})
	if err != nil {
		return nil, fmt.Errorf("configure OpenAI daily brief generator: %w", err)
	}
	return generator, nil
}

func runDailyBrief(
	ctx context.Context,
	sourceRecords recentSourceRecordsRepository,
	events dailyBriefEventsRepository,
	generator dailyBriefGenerator,
	stdout io.Writer,
	query dailyBriefInputQuery,
) error {
	input, err := assembleDailyBriefInput(ctx, sourceRecords, events, query)
	if err != nil {
		return fmt.Errorf("assemble daily brief input: %w", err)
	}

	brief, err := generateDailyBrief(ctx, generator, input)
	if err != nil {
		return fmt.Errorf("generate daily brief: %w", err)
	}

	output := newDailyBriefOutput(brief)
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("encode daily brief: %w", err)
	}
	return nil
}

func newDailyBriefOutput(brief dailyBrief) dailyBriefOutput {
	output := dailyBriefOutput{
		Region:   brief.region,
		Sections: make([]dailyBriefSectionOutput, 0, len(brief.sections)),
	}
	for _, section := range brief.sections {
		sectionOutput := dailyBriefSectionOutput{
			Heading:   section.heading,
			Content:   section.content,
			Citations: make([]dailyBriefCitationOutput, 0, len(section.citations)),
		}
		for _, citation := range section.citations {
			sectionOutput.Citations = append(sectionOutput.Citations, dailyBriefCitationOutput{
				Kind:   citation.kind,
				ID:     citation.id,
				Source: citation.source,
				URL:    citation.url,
			})
		}
		output.Sections = append(output.Sections, sectionOutput)
	}
	return output
}
