// Package app wires Atlas commands to infrastructure and application services.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

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
	BLS           CalendarSourceDependencies
	Fed           CalendarSourceDependencies
	Stdout        io.Writer
}

// Run executes one Atlas command.
func Run(ctx context.Context, arguments []string, dependencies Dependencies) error {
	if len(arguments) != 1 {
		return errors.New("usage: atlas <migrate|ingest-rss|ingest-bls|ingest-fed>")
	}
	if arguments[0] != "migrate" && arguments[0] != "ingest-rss" && arguments[0] != "ingest-bls" && arguments[0] != "ingest-fed" {
		return fmt.Errorf("unknown command %q; usage: atlas <migrate|ingest-rss|ingest-bls|ingest-fed>", arguments[0])
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
	switch arguments[0] {
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
	default:
		panic("validated command is not handled")
	}
}
