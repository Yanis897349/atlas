package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runCalendarIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	adapter calendar.Adapter,
	actor string,
	calendarName string,
	stdout io.Writer,
) error {
	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure calendar repository: %w", err)
	}

	count, err := calendar.Ingest(ctx, adapter, repository, actor)
	if err != nil {
		return fmt.Errorf("ingest %s: %w", calendarName, err)
	}
	_, _ = fmt.Fprintf(stdout, "ingested %d %s events\n", count, calendarName)
	return nil
}
