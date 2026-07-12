package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion/rss"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rssIngestionActor = "atlas-rss-ingestion"

func runRSSIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	embedder search.Embedder,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	feedURL := dependencies.RSSFeedURL
	if feedURL == "" {
		feedURL = rss.InvestingLiveFeedURL
	}
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  rss.InvestingLiveSource,
		FeedURL: feedURL,
		Client:  dependencies.RSSHTTPClient,
		Now:     dependencies.RSSNow,
		Wait:    dependencies.RSSWait,
	})
	if err != nil {
		return fmt.Errorf("configure InvestingLive RSS: %w", err)
	}
	count := 0
	if err := withRSSIngestionLock(ctx, pool, func(connection *pgxpool.Conn) error {
		sourceRepository, err := ingestionpostgres.NewRepository(connection)
		if err != nil {
			return fmt.Errorf("configure ingestion repository: %w", err)
		}
		embeddingRepository, err := searchpostgres.NewRepository(connection)
		if err != nil {
			return fmt.Errorf("configure source record embedding repository: %w", err)
		}

		storedRecords, err := ingestion.Ingest(ctx, adapter, sourceRepository, rssIngestionActor)
		if err != nil {
			return fmt.Errorf("ingest InvestingLive RSS: %w", err)
		}
		if _, err := search.IndexStoredSourceRecords(
			ctx,
			storedRecords,
			embedder,
			embeddingRepository,
			rssIngestionActor,
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
