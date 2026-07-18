package app

import (
	"strings"
	"testing"
)

func TestRunRejectsInvalidEconomicEventContextArgumentsBeforeConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{
			name: "event ID",
			arguments: []string{
				"economic-event-context", "--event-id", "not-a-uuid",
				"--from", "2026-07-12T08:00:00Z", "--to", "2026-07-12T12:00:00Z", "--limit", "10",
				"--observation-limit", "10",
				"--observation-revision-limit", "10",
			},
			contains: "--event-id must be a UUID",
		},
		{
			name: "publication window",
			arguments: []string{
				"economic-event-context", "--event-id", "00000000-0000-0000-0000-000000000085",
				"--from", "2026-07-12T12:00:00Z", "--to", "2026-07-12T08:00:00Z", "--limit", "10",
				"--observation-limit", "10",
				"--observation-revision-limit", "10",
			},
			contains: "--to must not be before --from",
		},
		{
			name: "limit",
			arguments: []string{
				"economic-event-context", "--event-id", "00000000-0000-0000-0000-000000000085",
				"--from", "2026-07-12T08:00:00Z", "--to", "2026-07-12T12:00:00Z", "--limit", "0",
				"--observation-limit", "10",
				"--observation-revision-limit", "10",
			},
			contains: "--limit must be between 1 and 100",
		},
		{
			name: "observation limit",
			arguments: []string{
				"economic-event-context", "--event-id", "00000000-0000-0000-0000-000000000085",
				"--from", "2026-07-12T08:00:00Z", "--to", "2026-07-12T12:00:00Z", "--limit", "10",
				"--observation-limit", "0",
				"--observation-revision-limit", "10",
			},
			contains: "--observation-limit must be between 1 and 100",
		},
		{
			name: "observation revision limit",
			arguments: []string{
				"economic-event-context", "--event-id", "00000000-0000-0000-0000-000000000085",
				"--from", "2026-07-12T08:00:00Z", "--to", "2026-07-12T12:00:00Z", "--limit", "10",
				"--observation-limit", "10", "--observation-revision-limit", "0",
			},
			contains: "--observation-revision-limit must be between 1 and 100",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Run(t.Context(), test.arguments, Dependencies{Getenv: func(string) string {
				t.Fatal("configuration read for invalid command arguments")
				return ""
			}})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(economic-event-context) error = %v, want containing %q", err, test.contains)
			}
		})
	}
}

func TestRunEconomicEventContextValidatesProviderConfigurationBeforeConnecting(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		contains string
	}{
		{
			name:     "missing API key",
			env:      map[string]string{"ATLAS_OPENAI_EMBEDDING_MODEL": "embedding-model"},
			contains: "OpenAI API key is required",
		},
		{
			name:     "missing embedding model",
			env:      map[string]string{"ATLAS_OPENAI_API_KEY": "secret"},
			contains: "OpenAI model is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.env["ATLAS_DATABASE_URL"] = "postgres://atlas:secret@127.0.0.1:1/atlas?sslmode=disable"
			err := Run(t.Context(), validEconomicEventContextArguments(), Dependencies{
				Getenv: func(name string) string { return test.env[name] },
			})
			if err == nil || !strings.Contains(err.Error(), "configure OpenAI source record embedder") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(economic-event-context) error = %v, want configuration error containing %q", err, test.contains)
			}
		})
	}
}

func validEconomicEventContextArguments() []string {
	return []string{
		"economic-event-context",
		"--event-id", "00000000-0000-0000-0000-000000000085",
		"--from", "2026-07-12T08:00:00Z",
		"--to", "2026-07-12T12:00:00Z",
		"--limit", "10",
		"--observation-limit", "10",
		"--observation-revision-limit", "10",
	}
}
