package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/fed"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
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
		CalendarURL:   dependencies.FedCalendarURL,
		Client:        dependencies.FedHTTPClient,
		Now:           dependencies.FedNow,
		RequestBudget: dependencies.FedRequestBudget,
	})
	if err != nil {
		return fmt.Errorf("configure Federal Reserve calendar: %w", err)
	}
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure calendar repository: %w", err)
	}

	count, err := calendar.Ingest(ctx, adapter, repository, fedIngestionActor)
	if err != nil {
		return fmt.Errorf("ingest Federal Reserve calendar: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "ingested %d Federal Reserve calendar events\n", count)
	return nil
}
