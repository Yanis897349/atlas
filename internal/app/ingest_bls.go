package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/bls"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
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
		CalendarURL:   dependencies.BLSCalendarURL,
		Client:        dependencies.BLSHTTPClient,
		Now:           dependencies.BLSNow,
		RequestBudget: dependencies.BLSRequestBudget,
	})
	if err != nil {
		return fmt.Errorf("configure BLS calendar: %w", err)
	}
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure calendar repository: %w", err)
	}

	count, err := calendar.Ingest(ctx, adapter, repository, blsIngestionActor)
	if err != nil {
		return fmt.Errorf("ingest BLS calendar: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "ingested %d BLS calendar events\n", count)
	return nil
}
