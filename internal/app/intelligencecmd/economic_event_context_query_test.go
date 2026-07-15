package intelligencecmd

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseEconomicEventContextNormalizesInput(t *testing.T) {
	command, recognized, err := Parse([]string{
		"economic-event-context",
		"--limit", "24",
		"--to", "2026-07-12T14:00:00+02:00",
		"--event-id", "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA",
		"--from", "2026-07-12T08:00:00Z",
	})
	if err != nil || !recognized {
		t.Fatalf("Parse() = (%#v, %t, %v), want recognized command", command, recognized, err)
	}
	wantStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	if command.name != "economic-event-context" ||
		command.query.EventID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" ||
		command.query.PublicationWindowStart != wantStart || command.query.PublicationWindowEnd != wantEnd ||
		command.query.SourceRecordLimit != 24 ||
		command.query.ObservationLimit != 100 {
		t.Errorf("command = %#v, want normalized complete query", command)
	}
}

func TestParseEconomicEventContextAcceptsEqualInclusiveWindow(t *testing.T) {
	arguments := replaceFlag(validEconomicEventContextArguments(), "--to", "2026-07-12T08:00:00Z")
	if _, _, err := Parse(arguments); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsInvalidEconomicEventContextArguments(t *testing.T) {
	valid := validEconomicEventContextArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing event ID", arguments: withoutFlag(valid, "--event-id"), contains: "--event-id is required"},
		{name: "missing from", arguments: withoutFlag(valid, "--from"), contains: "--from is required"},
		{name: "missing to", arguments: withoutFlag(valid, "--to"), contains: "--to is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "malformed event ID", arguments: replaceFlag(valid, "--event-id", "not-a-uuid"), contains: "--event-id must be a UUID"},
		{name: "invalid event ID separators", arguments: replaceFlag(valid, "--event-id", "00000000X0000X0000X0000X000000000085"), contains: "--event-id must be a UUID"},
		{name: "malformed from", arguments: replaceFlag(valid, "--from", "today"), contains: "--from must be RFC3339"},
		{name: "malformed to", arguments: replaceFlag(valid, "--to", "later"), contains: "--to must be RFC3339"},
		{name: "zero from", arguments: replaceFlag(valid, "--from", "0001-01-01T00:00:00Z"), contains: "--from must not be zero"},
		{name: "zero to", arguments: replaceFlag(valid, "--to", "0001-01-01T00:00:00Z"), contains: "--to must not be zero"},
		{name: "reversed window", arguments: replaceFlag(valid, "--to", "2026-07-12T07:59:59Z"), contains: "--to must not be before --from"},
		{name: "nonnumeric limit", arguments: replaceFlag(valid, "--limit", "many"), contains: "--limit must be between 1 and 100"},
		{name: "zero limit", arguments: replaceFlag(valid, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "high limit", arguments: replaceFlag(valid, "--limit", "101"), contains: "--limit must be between 1 and 100"},
		{name: "repeated event ID", arguments: append(valid, "--event-id", validEventID), contains: "must only be provided once"},
		{name: "repeated from", arguments: append(valid, "--from", "2026-07-12T09:00:00Z"), contains: "must only be provided once"},
		{name: "repeated to", arguments: append(valid, "--to", "2026-07-12T13:00:00Z"), contains: "must only be provided once"},
		{name: "repeated limit", arguments: append(valid, "--limit", "2"), contains: "must only be provided once"},
		{name: "unknown flag", arguments: append(valid, "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(valid, "extra"), contains: "unexpected positional arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, recognized, err := Parse(test.arguments)
			if !recognized {
				t.Fatal("Parse() did not recognize intelligence command")
			}
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				!strings.Contains(err.Error(), economicEventContextUsage) {
				t.Fatalf("Parse() error = %v, want containing %q and usage", err, test.contains)
			}
		})
	}
}

func TestParseDoesNotRecognizeOtherCommands(t *testing.T) {
	for _, arguments := range [][]string{nil, {"migrate"}, {"search-source-records"}} {
		command, recognized, err := Parse(arguments)
		if err != nil || recognized || !reflect.DeepEqual(command, Command{}) {
			t.Errorf("Parse(%q) = (%#v, %t, %v), want zero command, false, nil", arguments, command, recognized, err)
		}
	}
}

func validEconomicEventContextArguments() []string {
	return []string{
		"economic-event-context",
		"--event-id", validEventID,
		"--from", "2026-07-12T08:00:00Z",
		"--to", "2026-07-12T12:00:00Z",
		"--limit", "10",
	}
}

func withoutFlag(arguments []string, name string) []string {
	result := make([]string, 0, len(arguments)-2)
	for index := 0; index < len(arguments); index++ {
		if arguments[index] == name {
			index++
			continue
		}
		result = append(result, arguments[index])
	}
	return result
}

func replaceFlag(arguments []string, name, value string) []string {
	result := append([]string(nil), arguments...)
	for index := range result {
		if result[index] == name {
			result[index+1] = value
			return result
		}
	}
	panic("flag not found")
}
