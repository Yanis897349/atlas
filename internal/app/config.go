package app

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxDatabaseConnections = 4
	databaseConnectTimeout = 10 * time.Second
	databaseMaxIdleTime    = 5 * time.Minute
	databaseMaxLifetime    = 30 * time.Minute
)

func databaseConfig(databaseURL string) (*pgxpool.Config, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, errors.New("ATLAS_DATABASE_URL is required")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" {
		return nil, errors.New("ATLAS_DATABASE_URL must be an absolute PostgreSQL URL")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("ATLAS_DATABASE_URL must be a valid PostgreSQL URL")
	}
	config.MaxConns = maxDatabaseConnections
	config.MinConns = 0
	config.MaxConnIdleTime = databaseMaxIdleTime
	config.MaxConnLifetime = databaseMaxLifetime
	config.ConnConfig.ConnectTimeout = databaseConnectTimeout
	return config, nil
}
