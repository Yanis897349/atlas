package intelligencecmd

import (
	"reflect"
	"strings"
	"testing"
)

const (
	validCPIEventID        = "00000000-0000-0000-0000-000000000091"
	validEmploymentEventID = "00000000-0000-0000-0000-000000000092"
)

func TestParseIngestBLSObservationsNormalizesInput(t *testing.T) {
	command, recognized, err := Parse([]string{
		"ingest-bls-observations",
		"--employment-event-id", "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA",
		"--limit", "2",
		"--cpi-event-id", "BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB",
	})
	if err != nil || !recognized {
		t.Fatalf("Parse() = (%#v, %t, %v), want recognized command", command, recognized, err)
	}
	if command.name != "ingest-bls-observations" ||
		command.observationIngestion.cpiEventID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" ||
		command.observationIngestion.employmentEventID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" ||
		command.observationIngestion.limit != 2 ||
		command.RequiresSourceRecordEmbedder() || command.RequiresEventContextRepositories() {
		t.Errorf("command = %#v, want normalized BLS ingestion without event-context dependencies", command)
	}
	cpiEventID, employmentEventID, required := command.BLSObservationEventIDs()
	if !required || cpiEventID != command.observationIngestion.cpiEventID ||
		employmentEventID != command.observationIngestion.employmentEventID {
		t.Errorf("BLSObservationEventIDs() = (%q, %q, %t), want parsed bindings", cpiEventID, employmentEventID, required)
	}
}

func TestParseRejectsInvalidIngestBLSObservationsArguments(t *testing.T) {
	valid := validIngestBLSObservationsArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing CPI event", arguments: withoutFlag(valid, "--cpi-event-id"), contains: "--cpi-event-id is required"},
		{name: "missing employment event", arguments: withoutFlag(valid, "--employment-event-id"), contains: "--employment-event-id is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "invalid CPI event", arguments: replaceFlag(valid, "--cpi-event-id", "invalid"), contains: "--cpi-event-id must be a UUID"},
		{name: "invalid employment event", arguments: replaceFlag(valid, "--employment-event-id", "invalid"), contains: "--employment-event-id must be a UUID"},
		{name: "conflicting events", arguments: replaceFlag(valid, "--employment-event-id", validCPIEventID), contains: "must differ"},
		{name: "nonnumeric limit", arguments: replaceFlag(valid, "--limit", "many"), contains: "--limit must be between 1 and 100"},
		{name: "zero limit", arguments: replaceFlag(valid, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "high limit", arguments: replaceFlag(valid, "--limit", "101"), contains: "--limit must be between 1 and 100"},
		{name: "repeated CPI event", arguments: append(valid, "--cpi-event-id", validCPIEventID), contains: "must only be provided once"},
		{name: "repeated employment event", arguments: append(valid, "--employment-event-id", validEmploymentEventID), contains: "must only be provided once"},
		{name: "repeated limit", arguments: append(valid, "--limit", "1"), contains: "must only be provided once"},
		{name: "unknown flag", arguments: append(valid, "--actor", "operator"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(valid, "extra"), contains: "unexpected positional arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, recognized, err := Parse(test.arguments)
			if !recognized {
				t.Fatal("Parse() did not recognize BLS observation command")
			}
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				!strings.Contains(err.Error(), ingestBLSObservationsUsage) {
				t.Fatalf("Parse() error = %v, want containing %q and usage", err, test.contains)
			}
		})
	}
}

func TestCommandCapabilitiesExcludeOtherCommands(t *testing.T) {
	command, recognized, err := Parse(validEconomicEventContextArguments())
	if err != nil || !recognized {
		t.Fatalf("Parse() = (%#v, %t, %v), want event-context command", command, recognized, err)
	}
	if !command.RequiresSourceRecordEmbedder() || !command.RequiresEventContextRepositories() {
		t.Errorf("event-context capabilities = (%t, %t), want both true", command.RequiresSourceRecordEmbedder(), command.RequiresEventContextRepositories())
	}
	if cpiEventID, employmentEventID, required := command.BLSObservationEventIDs(); required || cpiEventID != "" || employmentEventID != "" {
		t.Errorf("BLSObservationEventIDs() = (%q, %q, %t), want empty", cpiEventID, employmentEventID, required)
	}
	if reflect.DeepEqual(command, Command{}) {
		t.Fatal("parsed event-context command is zero")
	}
}

func validIngestBLSObservationsArguments() []string {
	return []string{
		"ingest-bls-observations",
		"--cpi-event-id", validCPIEventID,
		"--employment-event-id", validEmploymentEventID,
		"--limit", "2",
	}
}
