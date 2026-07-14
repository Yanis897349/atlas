package app

import (
	"strings"
	"testing"
)

func TestRunRejectsInvalidSearchSourceRecordsArgumentsBeforeConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{
			name: "limit",
			arguments: []string{
				"search-source-records", "--query", "inflation", "--limit", "0",
			},
			contains: "--limit must be between 1 and 100",
		},
		{
			name: "blank source",
			arguments: []string{
				"search-source-records", "--query", "inflation", "--source", " \t", "--limit", "10",
			},
			contains: "--source must not be blank",
		},
		{
			name: "one-sided publication window",
			arguments: []string{
				"search-source-records", "--query", "inflation", "--from", "2026-07-12T08:00:00Z", "--limit", "10",
			},
			contains: "--from and --to must be supplied together",
		},
		{
			name: "malformed publication window",
			arguments: []string{
				"search-source-records", "--query", "inflation", "--from", "today", "--to", "2026-07-12T12:00:00Z", "--limit", "10",
			},
			contains: "--from must be RFC3339",
		},
		{
			name: "reversed publication window",
			arguments: []string{
				"search-source-records", "--query", "inflation", "--from", "2026-07-12T12:00:00Z", "--to", "2026-07-12T08:00:00Z", "--limit", "10",
			},
			contains: "--to must not be before --from",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Run(t.Context(), test.arguments, Dependencies{Getenv: func(string) string {
				t.Fatal("configuration read for invalid command arguments")
				return ""
			}})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(search-source-records) error = %v, want containing %q", err, test.contains)
			}
		})
	}
}

func TestRunSearchSourceRecordsValidatesProviderConfigurationBeforeConnecting(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		contains string
	}{
		{name: "missing API key", env: map[string]string{"ATLAS_OPENAI_EMBEDDING_MODEL": "embedding-model"}, contains: "OpenAI API key is required"},
		{name: "missing embedding model", env: map[string]string{"ATLAS_OPENAI_API_KEY": "secret"}, contains: "OpenAI model is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.env["ATLAS_DATABASE_URL"] = "postgres://atlas:secret@127.0.0.1:1/atlas?sslmode=disable"
			err := Run(t.Context(), validSearchSourceRecordsArguments(), Dependencies{
				Getenv: func(name string) string { return test.env[name] },
			})
			if err == nil || !strings.Contains(err.Error(), "configure OpenAI source record embedder") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(search-source-records) error = %v, want configuration error containing %q", err, test.contains)
			}
		})
	}
}

func validSearchSourceRecordsArguments() []string {
	return []string{"search-source-records", "--query", "inflation outlook", "--limit", "10"}
}
