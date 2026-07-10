package app

import (
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
	tests := [][]string{nil, {"unknown"}, {"migrate", "extra"}}
	for _, arguments := range tests {
		if err := Run(t.Context(), arguments, Dependencies{}); err == nil {
			t.Errorf("Run(%q) error = nil, want usage error", arguments)
		}
	}
}

func TestRunRequiresApplicationDatabaseURL(t *testing.T) {
	err := Run(t.Context(), []string{"migrate"}, Dependencies{
		Getenv: func(string) string { return "" },
	})
	if err == nil {
		t.Fatal("Run() error = nil, want missing ATLAS_DATABASE_URL error")
	}
}
