package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/searchcmd"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/search"
	searchopenai "github.com/Yanis897349/atlas/internal/search/openai"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func configuredSourceRecordEmbedder(
	required bool,
	getenv func(string) string,
	dependencies Dependencies,
) (search.Embedder, error) {
	if !required {
		return nil, nil
	}
	embedder, err := searchopenai.NewEmbedder(searchopenai.Config{
		APIKey:        getenv("ATLAS_OPENAI_API_KEY"),
		Model:         getenv("ATLAS_OPENAI_EMBEDDING_MODEL"),
		Client:        dependencies.OpenAIHTTPClient,
		Endpoint:      dependencies.OpenAIEmbeddingsEndpoint,
		RequestBudget: dependencies.OpenAIEmbeddingsRequestBudget,
	})
	if err != nil {
		return nil, fmt.Errorf("configure OpenAI source record embedder: %w", err)
	}
	return embedder, nil
}

func runSearchCommand(
	ctx context.Context,
	pool *pgxpool.Pool,
	embedder search.Embedder,
	stdout io.Writer,
	command searchcmd.Command,
) error {
	sourceRepository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure source record repository: %w", err)
	}
	embeddingRepository, err := searchpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure source record embedding repository: %w", err)
	}
	return searchcmd.Run(ctx, sourceRepository, embedder, embeddingRepository, stdout, command)
}
