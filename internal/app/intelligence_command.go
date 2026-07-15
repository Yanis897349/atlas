package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/intelligencecmd"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runIntelligenceCommand(
	ctx context.Context,
	pool *pgxpool.Pool,
	embedder search.Embedder,
	stdout io.Writer,
	command intelligencecmd.Command,
) error {
	eventRepository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure economic event repository: %w", err)
	}
	semanticRepository, err := searchpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure source record embedding repository: %w", err)
	}
	observationRepository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure economic event observation repository: %w", err)
	}
	return intelligencecmd.Run(
		ctx,
		eventRepository,
		observationRepository,
		embedder,
		semanticRepository,
		stdout,
		command,
	)
}
