// Package app wires Atlas commands to infrastructure and application services.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehttp"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion/rss"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CalendarSourceDependencies contains deterministic seams for one calendar source.
type CalendarSourceDependencies struct {
	HTTPClient    sourcehttp.Client
	CalendarURL   string
	Now           func() time.Time
	RequestBudget time.Duration
}

// Dependencies contains process-bound dependencies and deterministic test seams.
type Dependencies struct {
	Getenv        func(string) string
	RSSHTTPClient rss.HTTPClient
	RSSFeedURL    string
	RSSWait       func(context.Context, time.Duration) error
	BEA           CalendarSourceDependencies
	BLS           CalendarSourceDependencies
	ECB           CalendarSourceDependencies
	Fed           CalendarSourceDependencies
	Stdout        io.Writer
}

// Run executes one Atlas command.
func Run(ctx context.Context, arguments []string, dependencies Dependencies) error {
	parsedCommand, err := parseCommand(arguments)
	if err != nil {
		return err
	}

	getenv := dependencies.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	config, err := databaseConfig(getenv("ATLAS_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("configure PostgreSQL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}

	stdout := dependencies.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	switch parsedCommand.name {
	case "migrate":
		if err := databasepostgres.Migrate(ctx, pool); err != nil {
			return fmt.Errorf("migrate PostgreSQL: %w", err)
		}
		_, _ = fmt.Fprintln(stdout, "database migrations applied")
		return nil
	case "ingest-rss":
		return runRSSIngestion(ctx, pool, dependencies, stdout)
	case "ingest-bls":
		return runBLSIngestion(ctx, pool, dependencies, stdout)
	case "ingest-fed":
		return runFedIngestion(ctx, pool, dependencies, stdout)
	case "ingest-ecb":
		return runECBIngestion(ctx, pool, dependencies, stdout)
	case "ingest-bea":
		return runBEAIngestion(ctx, pool, dependencies, stdout)
	case "upcoming-events":
		repository, err := calendarpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure economic event repository: %w", err)
		}
		return runUpcomingEvents(ctx, repository, stdout, parsedCommand.upcomingEventsQuery)
	default:
		panic("validated command is not handled")
	}
}
