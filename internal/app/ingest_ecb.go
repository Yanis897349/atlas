package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar/ecb"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ecbIngestionActor = "atlas-ecb-calendar-ingestion"

func runECBIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	adapter, err := ecb.NewAdapter(ecb.Config{
		CalendarURL:   dependencies.ECB.CalendarURL,
		Client:        dependencies.ECB.HTTPClient,
		Now:           dependencies.ECB.Now,
		RequestBudget: dependencies.ECB.RequestBudget,
	})
	if err != nil {
		return fmt.Errorf("configure ECB calendar: %w", err)
	}
	return runCalendarIngestion(ctx, pool, adapter, ecbIngestionActor, "ECB calendar", stdout)
}
