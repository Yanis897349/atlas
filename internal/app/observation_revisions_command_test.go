package app

import (
	"strings"
	"testing"
)

func TestRunRejectsInvalidObservationRevisionsArgumentsBeforeConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{
			name: "event ID",
			arguments: []string{
				"economic-event-observation-revisions",
				"--event-id", "not-a-uuid",
				"--source", "official-statistics",
				"--source-observation-id", "cpi-2026-07",
				"--limit", "10",
			},
			contains: "--event-id must be a UUID",
		},
		{
			name: "source",
			arguments: []string{
				"economic-event-observation-revisions",
				"--event-id", "00000000-0000-0000-0000-000000000098",
				"--source", " ",
				"--source-observation-id", "cpi-2026-07",
				"--limit", "10",
			},
			contains: "--source must not be blank",
		},
		{
			name: "limit",
			arguments: []string{
				"economic-event-observation-revisions",
				"--event-id", "00000000-0000-0000-0000-000000000098",
				"--source", "official-statistics",
				"--source-observation-id", "cpi-2026-07",
				"--limit", "101",
			},
			contains: "--limit must be between 1 and 100",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Run(t.Context(), test.arguments, Dependencies{Getenv: func(string) string {
				t.Fatal("configuration read for invalid command arguments")
				return ""
			}})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run(economic-event-observation-revisions) error = %v, want containing %q", err, test.contains)
			}
		})
	}
}
