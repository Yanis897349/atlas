package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	rssIngestionLockReleaseBudget = 5 * time.Second
	acquireRSSIngestionLockSQL    = `
SELECT pg_advisory_lock(
    hashtext(current_database()),
    hashtext(current_schema() || ':atlas:rss:investinglive')
)`
	releaseRSSIngestionLockSQL = `
SELECT pg_advisory_unlock(
    hashtext(current_database()),
    hashtext(current_schema() || ':atlas:rss:investinglive')
)`
)

func withRSSIngestionLock(
	ctx context.Context,
	pool *pgxpool.Pool,
	operation func(*pgxpool.Conn) error,
) (resultErr error) {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire PostgreSQL connection for RSS ingestion: %w", err)
	}
	locked := false
	defer func() {
		if !locked {
			connection.Release()
			return
		}
		resultErr = errors.Join(resultErr, releaseRSSIngestionLock(connection))
	}()

	if _, err := connection.Exec(ctx, acquireRSSIngestionLockSQL); err != nil {
		return fmt.Errorf("acquire InvestingLive RSS ingestion lock: %w", err)
	}
	locked = true
	return operation(connection)
}

func releaseRSSIngestionLock(connection *pgxpool.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), rssIngestionLockReleaseBudget)
	defer cancel()

	var unlocked bool
	err := connection.QueryRow(ctx, releaseRSSIngestionLockSQL).Scan(&unlocked)
	if err == nil && unlocked {
		connection.Release()
		return nil
	}

	rawConnection := connection.Hijack()
	closeContext, closeCancel := context.WithTimeout(context.Background(), rssIngestionLockReleaseBudget)
	defer closeCancel()
	closeErr := rawConnection.Close(closeContext)
	if err != nil {
		return errors.Join(fmt.Errorf("release InvestingLive RSS ingestion lock: %w", err), closeErr)
	}
	return errors.Join(errors.New("release InvestingLive RSS ingestion lock: lock was not held"), closeErr)
}
