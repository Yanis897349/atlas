package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/intelligencecmd"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runIntelligenceCommand(
	ctx context.Context,
	pool *pgxpool.Pool,
	observationAdapter intelligence.ObservationAdapter,
	embedder search.Embedder,
	stdout io.Writer,
	command intelligencecmd.Command,
) error {
	eventRepository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure economic event repository: %w", err)
	}
	observationRepository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure economic event observation repository: %w", err)
	}
	commandDependencies := intelligencecmd.Dependencies{
		Events:                 eventRepository,
		Observations:           observationRepository,
		ObservationPersistence: observationRepository,
		ObservationAdapter:     observationAdapter,
		Embedder:               embedder,
	}
	if command.RequiresEventContextRepositories() {
		semanticRepository, err := searchpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure source record embedding repository: %w", err)
		}
		commandDependencies.SourceRecords = semanticRepository
	}
	return intelligencecmd.Run(ctx, commandDependencies, stdout, command)
}
