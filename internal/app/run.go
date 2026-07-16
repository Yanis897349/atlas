// Package app wires Atlas commands to infrastructure and application services.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Yanis897349/atlas/internal/app/watchlistcmd"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehttp"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	dailybriefpostgres "github.com/Yanis897349/atlas/internal/dailybrief/postgres"
	databasepostgres "github.com/Yanis897349/atlas/internal/database/postgres"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/ingestion/rss"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencebls "github.com/Yanis897349/atlas/internal/intelligence/bls"
	openaiapi "github.com/Yanis897349/atlas/internal/openai"
	"github.com/Yanis897349/atlas/internal/watchlist"
	watchlistpostgres "github.com/Yanis897349/atlas/internal/watchlist/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CalendarSourceDependencies contains deterministic seams for one calendar source.
type CalendarSourceDependencies struct {
	HTTPClient    sourcehttp.Client
	CalendarURL   string
	Now           func() time.Time
	RequestBudget time.Duration
}

// BLSObservationDependencies contains deterministic seams for official BLS observations.
type BLSObservationDependencies struct {
	HTTPClient    intelligencebls.HTTPClient
	Endpoint      string
	Now           func() time.Time
	RequestBudget time.Duration
}

// OpenAIHTTPClient executes OpenAI API requests.
type OpenAIHTTPClient = openaiapi.HTTPClient

// Dependencies contains process-bound dependencies and deterministic test seams.
type Dependencies struct {
	Getenv                        func(string) string
	RSSHTTPClient                 rss.HTTPClient
	RSSFeedURL                    string
	RSSNow                        func() time.Time
	RSSWait                       func(context.Context, time.Duration) error
	BEA                           CalendarSourceDependencies
	BLS                           CalendarSourceDependencies
	Census                        CalendarSourceDependencies
	ECB                           CalendarSourceDependencies
	Eurostat                      CalendarSourceDependencies
	Fed                           CalendarSourceDependencies
	SPGlobal                      CalendarSourceDependencies
	BLSObservations               BLSObservationDependencies
	OpenAIHTTPClient              OpenAIHTTPClient
	OpenAIEndpoint                string
	OpenAIRequestBudget           time.Duration
	OpenAIEmbeddingsEndpoint      string
	OpenAIEmbeddingsRequestBudget time.Duration
	Stdout                        io.Writer
}

// Run executes one Atlas command.
func Run(ctx context.Context, arguments []string, dependencies Dependencies) error {
	parsedCommand, err := parseCommand(arguments)
	if err != nil {
		return err
	}

	getenv := dependencies.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	config, err := databaseConfig(getenv("ATLAS_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("configure PostgreSQL: %w", err)
	}

	var dailyBriefGenerator dailybrief.Generator
	if parsedCommand.name == "daily-brief" {
		dailyBriefGenerator, err = configureOpenAIDailyBriefGenerator(getenv, dependencies)
		if err != nil {
			return err
		}
	}
	var observationAdapter intelligence.ObservationAdapter
	if parsedCommand.intelligenceCommand != nil {
		cpiEventID, employmentEventID, required := parsedCommand.intelligenceCommand.BLSObservationEventIDs()
		if required {
			observationAdapter, err = intelligencebls.NewAdapter(intelligencebls.Config{
				Targets: []intelligencebls.Target{
					{EconomicEventID: cpiEventID, Series: intelligencebls.SeriesCPIAllItemsNSA},
					{EconomicEventID: employmentEventID, Series: intelligencebls.SeriesTotalNonfarmPayrollSA},
				},
				Endpoint:      dependencies.BLSObservations.Endpoint,
				Client:        dependencies.BLSObservations.HTTPClient,
				Now:           dependencies.BLSObservations.Now,
				RequestBudget: dependencies.BLSObservations.RequestBudget,
			})
			if err != nil {
				return fmt.Errorf("configure BLS economic event observations: %w", err)
			}
		}
	}
	requiresIntelligenceEmbedder := parsedCommand.intelligenceCommand != nil &&
		parsedCommand.intelligenceCommand.RequiresSourceRecordEmbedder()
	sourceRecordEmbedder, err := configuredSourceRecordEmbedder(
		parsedCommand.searchCommand != nil || requiresIntelligenceEmbedder || parsedCommand.name == "ingest-rss",
		getenv,
		dependencies,
	)
	if err != nil {
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}

	stdout := dependencies.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	if parsedCommand.calendarIngestionCommand != nil {
		return runCalendarIngestion(
			ctx,
			pool,
			parsedCommand.calendarIngestionCommand,
			dependencies,
			stdout,
		)
	}
	if parsedCommand.intelligenceCommand != nil {
		return runIntelligenceCommand(
			ctx,
			pool,
			observationAdapter,
			sourceRecordEmbedder,
			stdout,
			*parsedCommand.intelligenceCommand,
		)
	}
	if parsedCommand.watchlistCommand != nil {
		repository, err := watchlistpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure watchlist repository: %w", err)
		}
		var candidates watchlist.EventCandidateReader
		if parsedCommand.watchlistCommand.RequiresEventCandidates() {
			eventRepository, err := calendarpostgres.NewRepository(pool)
			if err != nil {
				return fmt.Errorf("configure economic event repository: %w", err)
			}
			candidates = eventRepository
		}
		return watchlistcmd.Run(ctx, repository, candidates, stdout, *parsedCommand.watchlistCommand)
	}
	if parsedCommand.searchCommand != nil {
		return runSearchCommand(ctx, pool, sourceRecordEmbedder, stdout, *parsedCommand.searchCommand)
	}
	switch parsedCommand.name {
	case "migrate":
		if err := databasepostgres.Migrate(ctx, pool); err != nil {
			return fmt.Errorf("migrate PostgreSQL: %w", err)
		}
		_, _ = fmt.Fprintln(stdout, "database migrations applied")
		return nil
	case "ingest-rss":
		return runRSSIngestion(ctx, pool, sourceRecordEmbedder, dependencies, stdout)
	case "upcoming-events":
		repository, err := calendarpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure economic event repository: %w", err)
		}
		return runUpcomingEvents(ctx, repository, stdout, parsedCommand.upcomingEventsQuery)
	case "daily-brief-input":
		sourceRepository, err := ingestionpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure source record repository: %w", err)
		}
		eventRepository, err := calendarpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure economic event repository: %w", err)
		}
		return runDailyBriefInput(
			ctx,
			sourceRepository,
			eventRepository,
			stdout,
			parsedCommand.dailyBriefInputQuery,
		)
	case "daily-brief":
		sourceRepository, err := ingestionpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure source record repository: %w", err)
		}
		eventRepository, err := calendarpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure economic event repository: %w", err)
		}
		briefRepository, err := dailybriefpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure daily brief repository: %w", err)
		}
		return runDailyBrief(
			ctx,
			sourceRepository,
			eventRepository,
			dailyBriefGenerator,
			briefRepository,
			stdout,
			parsedCommand.dailyBriefInputQuery,
		)
	case "daily-briefs":
		briefRepository, err := dailybriefpostgres.NewRepository(pool)
		if err != nil {
			return fmt.Errorf("configure daily brief repository: %w", err)
		}
		return runStoredDailyBriefs(
			ctx,
			briefRepository,
			stdout,
			parsedCommand.storedDailyBriefsQuery,
		)
	default:
		panic("validated command is not handled")
	}
}
