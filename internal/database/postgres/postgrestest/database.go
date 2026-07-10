// Package postgrestest provides isolated PostgreSQL databases for tests.
package postgrestest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDatabaseEnvironment = "ATLAS_TEST_DATABASE_URL"

// Database is an isolated PostgreSQL schema and its connection details.
type Database struct {
	URL  string
	Pool *pgxpool.Pool
}

// Open creates an empty isolated schema and registers its cleanup with test.
func Open(test testing.TB) Database {
	test.Helper()

	databaseURL := os.Getenv(testDatabaseEnvironment)
	if databaseURL == "" {
		test.Skip(testDatabaseEnvironment + " is not set")
	}

	adminPool, err := pgxpool.New(test.Context(), databaseURL)
	if err != nil {
		test.Fatalf("connect to test PostgreSQL: %v", err)
	}
	test.Cleanup(adminPool.Close)
	if err := adminPool.Ping(test.Context()); err != nil {
		test.Fatalf("ping test PostgreSQL: %v", err)
	}

	schema := "atlas_test_" + randomHex(test, 8)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(test.Context(), `CREATE SCHEMA `+identifier); err != nil {
		test.Fatalf("create test schema: %v", err)
	}
	test.Cleanup(func() {
		if _, err := adminPool.Exec(context.Background(), `DROP SCHEMA `+identifier+` CASCADE`); err != nil {
			test.Errorf("drop test schema: %v", err)
		}
	})

	isolatedURL := withSearchPath(test, databaseURL, schema)
	pool, err := pgxpool.New(test.Context(), isolatedURL)
	if err != nil {
		test.Fatalf("connect to isolated test schema: %v", err)
	}
	test.Cleanup(pool.Close)
	if err := pool.Ping(test.Context()); err != nil {
		test.Fatalf("ping isolated test schema: %v", err)
	}

	return Database{URL: isolatedURL, Pool: pool}
}

func withSearchPath(test testing.TB, databaseURL, schema string) string {
	test.Helper()

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		test.Fatalf("parse test database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func randomHex(test testing.TB, size int) string {
	test.Helper()

	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		test.Fatalf("generate test schema name: %v", err)
	}
	return hex.EncodeToString(value)
}
