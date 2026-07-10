// Package app wires Atlas commands to infrastructure and application services.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion/rss"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ingestionActor = "atlas-rss-ingestion"

// Dependencies contains process-bound dependencies and deterministic test seams.
type Dependencies struct {
	Getenv     func(string) string
	HTTPClient rss.HTTPClient
	FeedURL    string
	RSSWait    func(context.Context, time.Duration) error
	Stdout     io.Writer
}

// Run executes one Atlas command.
func Run(ctx context.Context, arguments []string, dependencies Dependencies) error {
	if len(arguments) != 1 {
		return errors.New("usage: atlas <migrate|ingest-rss>")
	}
	if arguments[0] != "migrate" && arguments[0] != "ingest-rss" {
		return fmt.Errorf("unknown command %q; usage: atlas <migrate|ingest-rss>", arguments[0])
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
	default:
		panic("validated command is not handled")
	}
}

func runRSSIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	feedURL := dependencies.FeedURL
	if feedURL == "" {
		feedURL = rss.InvestingLiveFeedURL
	}
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  rss.InvestingLiveSource,
		FeedURL: feedURL,
		Client:  dependencies.HTTPClient,
		Wait:    dependencies.RSSWait,
	})
	if err != nil {
		return fmt.Errorf("configure InvestingLive RSS: %w", err)
	}
	repository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure ingestion repository: %w", err)
	}

	count, err := ingestion.Ingest(ctx, adapter, repository, ingestionActor)
	if err != nil {
		return fmt.Errorf("ingest InvestingLive RSS: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "ingested %d InvestingLive RSS records\n", count)
	return nil
}
