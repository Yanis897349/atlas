package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion/rss"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rssIngestionActor = "atlas-rss-ingestion"

func runRSSIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
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
		Wait:    dependencies.RSSWait,
	})
	if err != nil {
		return fmt.Errorf("configure InvestingLive RSS: %w", err)
	}
	repository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure ingestion repository: %w", err)
	}

	count, err := ingestion.Ingest(ctx, adapter, repository, rssIngestionActor)
	if err != nil {
		return fmt.Errorf("ingest InvestingLive RSS: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "ingested %d InvestingLive RSS records\n", count)
	return nil
}
