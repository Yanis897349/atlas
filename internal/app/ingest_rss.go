package app

import (
	"context"
	"io"

	"github.com/Yanis897349/atlas/internal/app/rsscmd"
	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runRSSIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	embedder search.Embedder,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	return rsscmd.Run(ctx, pool, embedder, stdout, rsscmd.Dependencies{
		HTTPClient: dependencies.RSSHTTPClient,
		FeedURL:    dependencies.RSSFeedURL,
		Now:        dependencies.RSSNow,
		Wait:       dependencies.RSSWait,
	})
}
