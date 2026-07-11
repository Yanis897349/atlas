package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/bea"
	"github.com/Yanis897349/atlas/internal/calendar/bls"
	"github.com/Yanis897349/atlas/internal/calendar/ecb"
	"github.com/Yanis897349/atlas/internal/calendar/eurostat"
	"github.com/Yanis897349/atlas/internal/calendar/fed"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	beaIngestionActor      = "atlas-bea-calendar-ingestion"
	blsIngestionActor      = "atlas-bls-calendar-ingestion"
	ecbIngestionActor      = "atlas-ecb-calendar-ingestion"
	eurostatIngestionActor = "atlas-eurostat-calendar-ingestion"
	fedIngestionActor      = "atlas-fed-calendar-ingestion"
)

type calendarIngestionCommand struct {
	name         string
	calendarName string
	actor        string
	newAdapter   func(Dependencies) (calendar.Adapter, error)
}

var calendarIngestionCommands = []calendarIngestionCommand{
	{
		name:         "ingest-bls",
		calendarName: "BLS calendar",
		actor:        blsIngestionActor,
		newAdapter: func(dependencies Dependencies) (calendar.Adapter, error) {
			return bls.NewAdapter(bls.Config{
				CalendarURL:   dependencies.BLS.CalendarURL,
				Client:        dependencies.BLS.HTTPClient,
				Now:           dependencies.BLS.Now,
				RequestBudget: dependencies.BLS.RequestBudget,
			})
		},
	},
	{
		name:         "ingest-fed",
		calendarName: "Federal Reserve calendar",
		actor:        fedIngestionActor,
		newAdapter: func(dependencies Dependencies) (calendar.Adapter, error) {
			return fed.NewAdapter(fed.Config{
				CalendarURL:   dependencies.Fed.CalendarURL,
				Client:        dependencies.Fed.HTTPClient,
				Now:           dependencies.Fed.Now,
				RequestBudget: dependencies.Fed.RequestBudget,
			})
		},
	},
	{
		name:         "ingest-ecb",
		calendarName: "ECB calendar",
		actor:        ecbIngestionActor,
		newAdapter: func(dependencies Dependencies) (calendar.Adapter, error) {
			return ecb.NewAdapter(ecb.Config{
				CalendarURL:   dependencies.ECB.CalendarURL,
				Client:        dependencies.ECB.HTTPClient,
				Now:           dependencies.ECB.Now,
				RequestBudget: dependencies.ECB.RequestBudget,
			})
		},
	},
	{
		name:         "ingest-bea",
		calendarName: "BEA calendar",
		actor:        beaIngestionActor,
		newAdapter: func(dependencies Dependencies) (calendar.Adapter, error) {
			return bea.NewAdapter(bea.Config{
				CalendarURL:   dependencies.BEA.CalendarURL,
				Client:        dependencies.BEA.HTTPClient,
				Now:           dependencies.BEA.Now,
				RequestBudget: dependencies.BEA.RequestBudget,
			})
		},
	},
	{
		name:         "ingest-eurostat",
		calendarName: "Eurostat calendar",
		actor:        eurostatIngestionActor,
		newAdapter: func(dependencies Dependencies) (calendar.Adapter, error) {
			return eurostat.NewAdapter(eurostat.Config{
				EventsURL:     dependencies.Eurostat.CalendarURL,
				Client:        dependencies.Eurostat.HTTPClient,
				Now:           dependencies.Eurostat.Now,
				RequestBudget: dependencies.Eurostat.RequestBudget,
			})
		},
	},
}

func findCalendarIngestionCommand(name string) *calendarIngestionCommand {
	for index := range calendarIngestionCommands {
		if calendarIngestionCommands[index].name == name {
			return &calendarIngestionCommands[index]
		}
	}
	return nil
}

func runCalendarIngestion(
	ctx context.Context,
	pool *pgxpool.Pool,
	command *calendarIngestionCommand,
	dependencies Dependencies,
	stdout io.Writer,
) error {
	adapter, err := command.newAdapter(dependencies)
	if err != nil {
		return fmt.Errorf("configure %s: %w", command.calendarName, err)
	}

	repository, err := calendarpostgres.NewRepository(pool)
	if err != nil {
		return fmt.Errorf("configure calendar repository: %w", err)
	}

	count, err := calendar.Ingest(ctx, adapter, repository, command.actor)
	if err != nil {
		return fmt.Errorf("ingest %s: %w", command.calendarName, err)
	}
	_, _ = fmt.Fprintf(stdout, "ingested %d %s events\n", count, command.calendarName)
	return nil
}
