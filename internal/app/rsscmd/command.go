// Package rsscmd executes the InvestingLive RSS ingestion command.
package rsscmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion/rss"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ingestionActor = "atlas-rss-ingestion"

// Dependencies contains deterministic seams for the RSS source adapter.
type Dependencies struct {
	HTTPClient rss.HTTPClient
	FeedURL    string
	Now        func() time.Time
	Wait       func(context.Context, time.Duration) error
}

// Run ingests and indexes one serialized InvestingLive RSS cycle.
func Run(
	ctx context.Context,
	pool *pgxpool.Pool,
	embedder search.Embedder,
	stdout io.Writer,
	dependencies Dependencies,
) error {
	feedURL := dependencies.FeedURL
	if feedURL == "" {
		feedURL = rss.InvestingLiveFeedURL
	}
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  rss.InvestingLiveSource,
		FeedURL: feedURL,
		Client:  dependencies.HTTPClient,
		Now:     dependencies.Now,
		Wait:    dependencies.Wait,
	})
	if err != nil {
		return fmt.Errorf("configure InvestingLive RSS: %w", err)
	}

	count := 0
	if err := withIngestionLock(ctx, pool, func(connection *pgxpool.Conn) error {
		sourceRepository, err := ingestionpostgres.NewRepository(connection)
		if err != nil {
			return fmt.Errorf("configure ingestion repository: %w", err)
		}
		embeddingRepository, err := searchpostgres.NewRepository(connection)
		if err != nil {
			return fmt.Errorf("configure source record embedding repository: %w", err)
		}

		storedRecords, err := ingestion.Ingest(ctx, adapter, sourceRepository, ingestionActor)
		if err != nil {
			return fmt.Errorf("ingest InvestingLive RSS: %w", err)
		}
		if _, err := search.IndexStoredSourceRecords(
			ctx,
			storedRecords,
			embedder,
			embeddingRepository,
			ingestionActor,
		); err != nil {
			return fmt.Errorf("index ingested InvestingLive RSS source records: %w", err)
		}
		count = len(storedRecords)
		return nil
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "ingested %d InvestingLive RSS records\n", count)
	return nil
}
