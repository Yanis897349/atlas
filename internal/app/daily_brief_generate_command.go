package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

const dailyBriefGenerationActor = "daily-brief-generation"

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
	briefs dailyBriefPersistence,
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
	stored, err := briefs.PersistDailyBrief(ctx, brief, dailyBriefGenerationActor)
	if err != nil {
		return fmt.Errorf("persist daily brief: %w", err)
	}

	output := newDailyBriefOutput(stored)
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("encode daily brief: %w", err)
	}
	return nil
}
