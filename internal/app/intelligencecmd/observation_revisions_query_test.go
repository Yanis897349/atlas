package intelligencecmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseObservationRevisionsQueryNormalizesInput(t *testing.T) {
	command, recognized, err := Parse([]string{
		"economic-event-observation-revisions",
		"--source-observation-id", " CPIAUCSL-2026-M06 ",
		"--limit", "24",
		"--event-id", "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA",
		"--source", " BLS-Official ",
	})
	if err != nil || !recognized {
		t.Fatalf("Parse() = (%#v, %t, %v), want recognized command", command, recognized, err)
	}
	want := observationRevisionsQuery{
		eventID:             "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		source:              "BLS-Official",
		sourceObservationID: "CPIAUCSL-2026-M06",
		limit:               24,
	}
	if command.name != "economic-event-observation-revisions" ||
		!reflect.DeepEqual(command.observationRevisionsQuery, want) ||
		command.RequiresSourceRecordEmbedder() || command.RequiresEventContextRepositories() {
		t.Errorf("command = %#v, want normalized revision query without provider dependencies", command)
	}
	if targets, required := command.BLSObservationTargets(); required || targets != nil {
		t.Errorf("BLSObservationTargets() = (%#v, %t), want (nil, false)", targets, required)
	}
}

func TestParseRejectsInvalidObservationRevisionsArguments(t *testing.T) {
	valid := validObservationRevisionsArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing event ID", arguments: withoutFlag(valid, "--event-id"), contains: "--event-id is required"},
		{name: "missing source", arguments: withoutFlag(valid, "--source"), contains: "--source is required"},
		{name: "missing source observation ID", arguments: withoutFlag(valid, "--source-observation-id"), contains: "--source-observation-id is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "malformed event ID", arguments: replaceFlag(valid, "--event-id", "not-a-uuid"), contains: "--event-id must be a UUID"},
		{name: "invalid event ID separators", arguments: replaceFlag(valid, "--event-id", "00000000X0000X0000X0000X000000000098"), contains: "--event-id must be a UUID"},
		{name: "blank source", arguments: replaceFlag(valid, "--source", " \t"), contains: "--source must not be blank"},
		{name: "blank source observation ID", arguments: replaceFlag(valid, "--source-observation-id", " "), contains: "--source-observation-id must not be blank"},
		{name: "nonnumeric limit", arguments: replaceFlag(valid, "--limit", "many"), contains: "--limit must be between 1 and 100"},
		{name: "zero limit", arguments: replaceFlag(valid, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "negative limit", arguments: replaceFlag(valid, "--limit", "-1"), contains: "--limit must be between 1 and 100"},
		{name: "high limit", arguments: replaceFlag(valid, "--limit", "101"), contains: "--limit must be between 1 and 100"},
		{name: "repeated event ID", arguments: append(valid, "--event-id", validEventID), contains: "must only be provided once"},
		{name: "repeated source", arguments: append(valid, "--source", "other"), contains: "must only be provided once"},
		{name: "repeated source observation ID", arguments: append(valid, "--source-observation-id", "other"), contains: "must only be provided once"},
		{name: "repeated limit", arguments: append(valid, "--limit", "1"), contains: "must only be provided once"},
		{name: "unknown flag", arguments: append(valid, "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(valid, "extra"), contains: "unexpected positional arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, recognized, err := Parse(test.arguments)
			if !recognized {
				t.Fatal("Parse() did not recognize observation revisions command")
			}
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				!strings.Contains(err.Error(), observationRevisionsUsage) {
				t.Fatalf("Parse() error = %v, want containing %q and usage", err, test.contains)
			}
		})
	}
}

func validObservationRevisionsArguments() []string {
	return []string{
		"economic-event-observation-revisions",
		"--event-id", validEventID,
		"--source", "official-statistics",
		"--source-observation-id", "cpi-2026-07",
		"--limit", "10",
	}
}
