package rsscmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ingestionLockReleaseBudget = 5 * time.Second
	acquireIngestionLockSQL    = `
SELECT pg_advisory_lock(
    hashtext(current_database()),
    hashtext(current_schema() || ':atlas:rss:investinglive')
)`
	releaseIngestionLockSQL = `
SELECT pg_advisory_unlock(
    hashtext(current_database()),
    hashtext(current_schema() || ':atlas:rss:investinglive')
)`
)

func withIngestionLock(
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
		resultErr = errors.Join(resultErr, releaseIngestionLock(connection))
	}()

	if _, err := connection.Exec(ctx, acquireIngestionLockSQL); err != nil {
		return fmt.Errorf("acquire InvestingLive RSS ingestion lock: %w", err)
	}
	locked = true
	return operation(connection)
}

func releaseIngestionLock(connection *pgxpool.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), ingestionLockReleaseBudget)
	defer cancel()

	var unlocked bool
	err := connection.QueryRow(ctx, releaseIngestionLockSQL).Scan(&unlocked)
	if err == nil && unlocked {
		connection.Release()
		return nil
	}

	rawConnection := connection.Hijack()
	closeContext, closeCancel := context.WithTimeout(context.Background(), ingestionLockReleaseBudget)
	defer closeCancel()
	closeErr := rawConnection.Close(closeContext)
	if err != nil {
		return errors.Join(fmt.Errorf("release InvestingLive RSS ingestion lock: %w", err), closeErr)
	}
	return errors.Join(errors.New("release InvestingLive RSS ingestion lock: lock was not held"), closeErr)
}
