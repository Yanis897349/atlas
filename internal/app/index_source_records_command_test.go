package app

import (
	"strings"
	"testing"
)

func TestRunRejectsInvalidIndexSourceRecordsArgumentsBeforeConfiguration(t *testing.T) {
	arguments := validIndexSourceRecordsArguments()
	arguments = append(arguments, "--limit", "11")
	err := Run(t.Context(), arguments, Dependencies{Getenv: func(string) string {
		t.Fatal("configuration read for invalid command arguments")
		return ""
	}})
	if err == nil || !strings.Contains(err.Error(), "must only be provided once") {
		t.Fatalf("Run(index-source-records) error = %v, want repeated flag error", err)
	}
}

func TestRunIndexSourceRecordsValidatesProviderConfigurationBeforeConnecting(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		contains string
	}{
		{name: "missing API key", env: map[string]string{"ATLAS_OPENAI_EMBEDDING_MODEL": "embedding-model"}, contains: "OpenAI API key is required"},
		{name: "missing embedding model", env: map[string]string{"ATLAS_OPENAI_API_KEY": "secret", "ATLAS_OPENAI_MODEL": "responses-model"}, contains: "OpenAI model is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.env["ATLAS_DATABASE_URL"] = "postgres://atlas:secret@127.0.0.1:1/atlas?sslmode=disable"
			err := Run(t.Context(), validIndexSourceRecordsArguments(), Dependencies{
				Getenv: func(name string) string { return test.env[name] },
			})
			if err == nil || !strings.Contains(err.Error(), "configure OpenAI source record embedder") ||
				!strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(index-source-records) error = %v, want configuration error containing %q", err, test.contains)
			}
		})
	}
}

func validIndexSourceRecordsArguments() []string {
	return []string{
		"index-source-records",
		"--from", "2026-07-12T08:00:00Z",
		"--to", "2026-07-12T12:00:00Z",
		"--limit", "10",
		"--actor", "indexer",
	}
}
