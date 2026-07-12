// Package postgres owns the Atlas PostgreSQL schema migrations.
package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/000001_create_source_records.up.sql
var sourceRecordsMigration string

//go:embed migrations/000002_create_economic_events.up.sql
var economicEventsMigration string

//go:embed migrations/000003_index_source_records_published_at.up.sql
var sourceRecordsPublishedAtIndexMigration string

//go:embed migrations/000004_create_daily_briefs.up.sql
var dailyBriefsMigration string

//go:embed migrations/000005_create_watchlists.up.sql
var watchlistsMigration string

//go:embed migrations/000006_create_watchlist_event_links.up.sql
var watchlistEventLinksMigration string

//go:embed migrations/000007_index_economic_events_scheduled_at.up.sql
var economicEventsScheduledAtIndexMigration string

var migrations = []struct {
	version int64
	query   string
}{
	{version: 1, query: sourceRecordsMigration},
	{version: 2, query: economicEventsMigration},
	{version: 3, query: sourceRecordsPublishedAtIndexMigration},
	{version: 4, query: dailyBriefsMigration},
	{version: 5, query: watchlistsMigration},
	{version: 6, query: watchlistEventLinksMigration},
	{version: 7, query: economicEventsScheduledAtIndexMigration},
}

// Migrate applies pending database migrations transactionally.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("PostgreSQL connection is required")
	}

	transaction, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migrations: %w", err)
	}
	defer func() {
		_ = transaction.Rollback(context.Background())
	}()

	if _, err := transaction.Exec(ctx, `SELECT pg_advisory_xact_lock(1096043603)`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := transaction.Exec(ctx, `
CREATE TABLE IF NOT EXISTS atlas_schema_migrations (
    version bigint PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT statement_timestamp()
)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	for _, migration := range migrations {
		var applied bool
		if err := transaction.QueryRow(
			ctx,
			`SELECT EXISTS (SELECT 1 FROM atlas_schema_migrations WHERE version = $1)`,
			migration.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", migration.version, err)
		}
		if applied {
			continue
		}

		if _, err := transaction.Exec(ctx, migration.query); err != nil {
			return fmt.Errorf("apply migration %d: %w", migration.version, err)
		}
		if _, err := transaction.Exec(
			ctx,
			`INSERT INTO atlas_schema_migrations (version) VALUES ($1)`,
			migration.version,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", migration.version, err)
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
