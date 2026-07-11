package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar/bea"
	"github.com/jackc/pgx/v5/pgxpool"
)

const beaIngestionActor = "atlas-bea-calendar-ingestion"

func runBEAIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	adapter, err := bea.NewAdapter(bea.Config{
		CalendarURL:   dependencies.BEA.CalendarURL,
		Client:        dependencies.BEA.HTTPClient,
		Now:           dependencies.BEA.Now,
		RequestBudget: dependencies.BEA.RequestBudget,
	})
	if err != nil {
		return fmt.Errorf("configure BEA calendar: %w", err)
	}
	return runCalendarIngestion(ctx, pool, adapter, beaIngestionActor, "BEA calendar", stdout)
}
