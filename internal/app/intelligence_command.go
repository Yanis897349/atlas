package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Yanis897349/atlas/internal/app/intelligencecmd"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencebls "github.com/Yanis897349/atlas/internal/intelligence/bls"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BLSObservationDependencies contains deterministic seams for official BLS observations.
type BLSObservationDependencies struct {
	HTTPClient    intelligencebls.HTTPClient
	Endpoint      string
	Now           func() time.Time
	RequestBudget time.Duration
}

func configuredBLSObservationAdapter(
	command *intelligencecmd.Command,
	dependencies BLSObservationDependencies,
) (intelligence.ObservationAdapter, error) {
	if command == nil {
		return nil, nil
	}
	targets, required := command.BLSObservationTargets()
	if !required {
		return nil, nil
	}
	adapter, err := intelligencebls.NewAdapter(intelligencebls.Config{
		Targets:       targets,
		Endpoint:      dependencies.Endpoint,
		Client:        dependencies.HTTPClient,
		Now:           dependencies.Now,
		RequestBudget: dependencies.RequestBudget,
	})
	if err != nil {
		return nil, fmt.Errorf("configure BLS economic event observations: %w", err)
	}
	return adapter, nil
}

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
		Events:               eventRepository,
		Observations:         observationRepository,
		ObservationRevisions: observationRepository,
		ObservationWriter:    observationRepository,
		ObservationAdapter:   observationAdapter,
		Embedder:             embedder,
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
