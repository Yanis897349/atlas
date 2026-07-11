package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/dailybrief"
	dailybriefopenai "github.com/Yanis897349/atlas/internal/dailybrief/openai"
)

const dailyBriefGenerationActor = "daily-brief-generation"

func configureOpenAIDailyBriefGenerator(
	getenv func(string) string,
	dependencies Dependencies,
) (dailybrief.Generator, error) {
	generator, err := dailybriefopenai.NewGenerator(dailybriefopenai.Config{
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
	sourceRecords dailybrief.SourceRecords,
	events dailybrief.Events,
	generator dailybrief.Generator,
	briefs dailybrief.Persistence,
	stdout io.Writer,
	query dailybrief.InputQuery,
) error {
	input, err := dailybrief.AssembleInput(ctx, sourceRecords, events, query)
	if err != nil {
		return fmt.Errorf("assemble daily brief input: %w", err)
	}

	brief, err := dailybrief.Generate(ctx, generator, input)
	if err != nil {
		return fmt.Errorf("generate daily brief: %w", err)
	}
	stored, err := briefs.PersistDailyBrief(ctx, brief, dailyBriefGenerationActor)
	if err != nil {
		return fmt.Errorf("persist daily brief: %w", err)
	}

	output := newDailyBriefOutput(stored)
	return encodeCommandJSON(stdout, "daily brief", output)
}
