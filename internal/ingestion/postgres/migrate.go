package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const sourceRecordsMigrationVersion = 1

//go:embed migrations/000001_create_source_records.up.sql
var sourceRecordsMigration string

// Migrate applies pending ingestion database migrations transactionally.
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

	var applied bool
	if err := transaction.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM atlas_schema_migrations WHERE version = $1)`,
		sourceRecordsMigrationVersion,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check migration %d: %w", sourceRecordsMigrationVersion, err)
	}
	if !applied {
		if _, err := transaction.Exec(ctx, sourceRecordsMigration); err != nil {
			return fmt.Errorf("apply migration %d: %w", sourceRecordsMigrationVersion, err)
		}
		if _, err := transaction.Exec(
			ctx,
			`INSERT INTO atlas_schema_migrations (version) VALUES ($1)`,
			sourceRecordsMigrationVersion,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", sourceRecordsMigrationVersion, err)
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
