package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar/bls"
	"github.com/jackc/pgx/v5/pgxpool"
)

const blsIngestionActor = "atlas-bls-calendar-ingestion"

func runBLSIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	adapter, err := bls.NewAdapter(bls.Config{
		CalendarURL:   dependencies.BLS.CalendarURL,
		Client:        dependencies.BLS.HTTPClient,
		Now:           dependencies.BLS.Now,
		RequestBudget: dependencies.BLS.RequestBudget,
	})
	if err != nil {
		return fmt.Errorf("configure BLS calendar: %w", err)
	}
	return runCalendarIngestion(ctx, pool, adapter, blsIngestionActor, "BLS calendar", stdout)
}
