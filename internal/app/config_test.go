package app

import (
	"strings"
	"testing"
)

func TestDatabaseConfigValidatesAndBoundsPool(t *testing.T) {
	for _, databaseURL := range []string{"", "not-a-postgres-url", "host=localhost dbname=atlas", "https://localhost/atlas"} {
		if _, err := databaseConfig(databaseURL); err == nil {
			t.Errorf("databaseConfig(%q) error = nil, want validation error", databaseURL)
		}
	}

	config, err := databaseConfig("postgres://atlas:secret@localhost:5432/atlas?sslmode=disable")
	if err != nil {
		t.Fatalf("databaseConfig() error = %v", err)
	}
	if config.MaxConns != maxDatabaseConnections || config.MinConns != 0 {
		t.Errorf("pool bounds = (%d, %d), want (%d, 0)", config.MaxConns, config.MinConns, maxDatabaseConnections)
	}
	if config.ConnConfig.ConnectTimeout != databaseConnectTimeout {
		t.Errorf("connect timeout = %v, want %v", config.ConnConfig.ConnectTimeout, databaseConnectTimeout)
	}
}

func TestRunValidatesCommandBeforeConfiguration(t *testing.T) {
	tests := [][]string{nil, {"unknown"}, {"migrate", "extra"}, {"ingest-bea", "extra"}, {"ingest-census", "extra"}, {"ingest-eurostat", "extra"}, {"ingest-spglobal", "extra"}}
	for _, arguments := range tests {
		if err := Run(t.Context(), arguments, Dependencies{}); err == nil {
			t.Errorf("Run(%q) error = nil, want usage error", arguments)
		}
	}
}

func TestRunValidatesWatchlistCommandBeforeConfiguration(t *testing.T) {
	err := Run(t.Context(), []string{"create-watchlist"}, Dependencies{Getenv: func(string) string {
		t.Fatal("configuration read for invalid watchlist command")
		return ""
	}})
	if err == nil {
		t.Fatal("Run(create-watchlist) error = nil, want argument error")
	}
}

func TestRunRecognizesCommandsBeforeRequiringApplicationDatabaseURL(t *testing.T) {
	commands := [][]string{
		{"migrate"},
		{"ingest-rss"},
		{"ingest-bls"},
		{"ingest-fed"},
		{"ingest-ecb"},
		{"ingest-bea"},
		{"ingest-census"},
		{"ingest-eurostat"},
		{"ingest-spglobal"},
		validBLSObservationArguments(),
	}
	for _, arguments := range commands {
		command := arguments[0]
		t.Run(command, func(t *testing.T) {
			err := Run(t.Context(), arguments, Dependencies{
				Getenv: func(string) string { return "" },
			})
			if err == nil || err.Error() != "configure PostgreSQL: ATLAS_DATABASE_URL is required" {
				t.Fatalf("Run(%q) error = %v, want missing ATLAS_DATABASE_URL error", command, err)
			}
		})
	}
}

func TestRunRejectsInvalidBLSObservationArgumentsBeforeConfiguration(t *testing.T) {
	arguments := validBLSObservationArguments()
	arguments[len(arguments)-1] = "0"
	err := Run(t.Context(), arguments, Dependencies{Getenv: func(string) string {
		t.Fatal("configuration read for invalid BLS observation arguments")
		return ""
	}})
	if err == nil || !strings.Contains(err.Error(), "--limit must be between 1 and 100") {
		t.Fatalf("Run(ingest-bls-observations) error = %v, want limit validation", err)
	}
}

func TestRunValidatesBLSObservationConfigurationBeforeConnecting(t *testing.T) {
	err := Run(t.Context(), validBLSObservationArguments(), Dependencies{
		Getenv: func(name string) string {
			if name == "ATLAS_DATABASE_URL" {
				return "postgres://atlas:secret@127.0.0.1:1/atlas?sslmode=disable"
			}
			return ""
		},
		BLSObservations: BLSObservationDependencies{Endpoint: "http://example.com/bls"},
	})
	if err == nil || !strings.Contains(err.Error(), "configure BLS economic event observations") ||
		!strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("Run(ingest-bls-observations) error = %v, want provider configuration before connection", err)
	}
}

func TestRunBLSObservationIngestionDoesNotRequireOpenAIConfiguration(t *testing.T) {
	err := Run(t.Context(), validBLSObservationArguments(), Dependencies{
		Getenv: func(name string) string {
			if name == "ATLAS_DATABASE_URL" {
				return "postgres://atlas:secret@127.0.0.1:1/atlas?sslmode=disable&connect_timeout=1"
			}
			return ""
		},
	})
	if err == nil || !strings.Contains(err.Error(), "connect PostgreSQL") || strings.Contains(err.Error(), "OpenAI") {
		t.Fatalf("Run(ingest-bls-observations) error = %v, want database connection failure without OpenAI validation", err)
	}
}

func validBLSObservationArguments() []string {
	return []string{
		"ingest-bls-observations",
		"--cpi-event-id", "00000000-0000-0000-0000-000000000091",
		"--employment-event-id", "00000000-0000-0000-0000-000000000092",
		"--limit", "2",
	}
}

func TestRunIngestRSSRequiresEmbeddingConfigurationBeforeConnecting(t *testing.T) {
	err := Run(t.Context(), []string{"ingest-rss"}, Dependencies{Getenv: func(name string) string {
		if name == "ATLAS_DATABASE_URL" {
			return "postgres://atlas:secret@localhost:1/atlas?sslmode=disable"
		}
		return ""
	}})
	if err == nil || !strings.Contains(err.Error(), "configure OpenAI source record embedder") ||
		!strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("Run(ingest-rss) error = %v, want embedding configuration failure before database connection", err)
	}
}
