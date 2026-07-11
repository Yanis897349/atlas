package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar/fed"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fedIngestionActor = "atlas-fed-calendar-ingestion"

func runFedIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	adapter, err := fed.NewAdapter(fed.Config{
		CalendarURL:   dependencies.Fed.CalendarURL,
		Client:        dependencies.Fed.HTTPClient,
		Now:           dependencies.Fed.Now,
		RequestBudget: dependencies.Fed.RequestBudget,
	})
	if err != nil {
		return fmt.Errorf("configure Federal Reserve calendar: %w", err)
	}
	return runCalendarIngestion(ctx, pool, adapter, fedIngestionActor, "Federal Reserve calendar", stdout)
}
