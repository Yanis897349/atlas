package intelligencecmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

func TestNewEconomicEventContextOutputMapsCompleteOrderedContext(t *testing.T) {
	paris := time.FixedZone("Paris", 2*60*60)
	windowStart := time.Date(2026, time.July, 12, 10, 0, 0, 0, paris)
	windowEnd := windowStart.Add(4 * time.Hour)
	event := storedEventFixture("  Consumer Price Index  ", windowEnd)
	latest := intelligence.StoredObservation{
		ID: "00000000-0000-0000-0000-000000000087",
		Observation: intelligence.Observation{
			EconomicEventID:     event.ID,
			Source:              "official-statistics",
			SourceObservationID: "cpi-2026-07",
			SourceURL:           "https://example.com/releases/cpi-2026-07",
			ObservedAt:          windowEnd.Add(time.Hour),
			Consensus:           observationValue("3.1%"),
			Previous:            observationValue("3.0%"),
			Actual:              observationValue("3.3%"),
		},
		CreatedAt: windowEnd.Add(2 * time.Hour),
		UpdatedAt: windowEnd.Add(3 * time.Hour),
		CreatedBy: "observation-ingestion",
		UpdatedBy: "observation-refresh",
	}
	secondary := intelligence.StoredObservation{
		ID: "00000000-0000-0000-0000-000000000086",
		Observation: intelligence.Observation{
			EconomicEventID:     event.ID,
			Source:              "secondary-statistics",
			SourceObservationID: "cpi-secondary-2026-07",
			SourceURL:           "https://example.org/releases/cpi-2026-07",
			ObservedAt:          windowStart,
			Previous:            observationValue("3.2%"),
		},
		CreatedAt: windowStart.Add(time.Hour),
		UpdatedAt: windowStart.Add(2 * time.Hour),
		CreatedBy: "secondary-ingestion",
		UpdatedBy: "secondary-refresh",
	}
	older := latest
	older.ID = "00000000-0000-0000-0000-000000000085"
	older.SourceURL = "https://example.com/releases/cpi-2026-07-initial"
	older.ObservedAt = windowEnd
	older.Actual = observationValue("3.10%")
	surprise := "+0.2%"
	direction := intelligence.SurpriseDirectionAboveConsensus
	delta := "+0.2%"
	context := intelligence.EventContext{
		Event:                  event,
		PublicationWindowStart: windowStart,
		PublicationWindowEnd:   windowEnd,
		Observations: []intelligence.EventContextObservation{
			{
				Latest:            latest,
				Surprise:          &surprise,
				SurpriseDirection: &direction,
				Revisions:         []intelligence.StoredObservation{latest, older},
				Comparisons: []intelligence.ObservationRevisionComparison{{
					NewerRevisionID: latest.ID,
					OlderRevisionID: older.ID,
					Changes: []intelligence.ObservationRevisionChange{
						{
							Field:    intelligence.ObservationRevisionFieldPrevious,
							NewValue: latest.Previous,
						},
						{
							Field:    intelligence.ObservationRevisionFieldActual,
							OldValue: older.Actual,
							NewValue: latest.Actual,
							Delta:    &delta,
						},
						{
							Field:    intelligence.ObservationRevisionFieldSourceURL,
							OldValue: &older.SourceURL,
							NewValue: &latest.SourceURL,
						},
					},
				}},
			},
			{
				Latest:      secondary,
				Revisions:   []intelligence.StoredObservation{},
				Comparisons: []intelligence.ObservationRevisionComparison{},
			},
		},
		SourceRecords: []search.SimilarSourceRecord{
			similarSourceRecordFixture(
				"00000000-0000-0000-0000-000000000002",
				"Second",
				windowStart,
				0.1,
			),
			similarSourceRecordFixture(
				"00000000-0000-0000-0000-000000000001",
				"First",
				windowStart.Add(time.Hour),
				0.4,
			),
		},
	}

	got := newEconomicEventContextOutput(context)

	wantEvent := economicEventOutput{
		ID:              validEventID,
		Source:          "official-calendar",
		ExternalEventID: "event-85",
		Name:            "  Consumer Price Index  ",
		Region:          event.Region,
		EventType:       event.Type,
		ScheduledAt:     "2026-07-12T12:00:00Z",
		SourceURL:       "https://example.com/calendar/event-85",
		RetrievedAt:     "2026-07-12T11:00:00Z",
		CreatedAt:       "2026-07-12T10:00:00Z",
		UpdatedAt:       "2026-07-12T11:00:00Z",
		CreatedBy:       "calendar-ingestion",
		UpdatedBy:       "calendar-refresh",
	}
	if !reflect.DeepEqual(got.Event, wantEvent) {
		t.Errorf("event = %#v, want %#v", got.Event, wantEvent)
	}
	if got.PublicationWindowStart != "2026-07-12T08:00:00Z" ||
		got.PublicationWindowEnd != "2026-07-12T12:00:00Z" || len(got.Observations) != 2 ||
		got.Observations[0].ID != latest.ID || got.Observations[1].ID != secondary.ID ||
		got.Observations[0].EconomicEventID != event.ID ||
		got.Observations[0].Source != "official-statistics" ||
		got.Observations[0].SourceObservationID != "cpi-2026-07" ||
		got.Observations[0].SourceURL != latest.SourceURL ||
		got.Observations[0].ObservedAt != "2026-07-12T13:00:00Z" ||
		got.Observations[0].Consensus == nil || *got.Observations[0].Consensus != "3.1%" ||
		got.Observations[0].Previous == nil || *got.Observations[0].Previous != "3.0%" ||
		got.Observations[0].Actual == nil || *got.Observations[0].Actual != "3.3%" ||
		got.Observations[0].Surprise == nil || *got.Observations[0].Surprise != surprise ||
		got.Observations[0].SurpriseDirection == nil ||
		*got.Observations[0].SurpriseDirection != direction ||
		got.Observations[0].CreatedAt != "2026-07-12T14:00:00Z" ||
		got.Observations[0].UpdatedAt != "2026-07-12T15:00:00Z" ||
		got.Observations[0].CreatedBy != "observation-ingestion" ||
		got.Observations[0].UpdatedBy != "observation-refresh" ||
		got.Observations[1].Consensus != nil || got.Observations[1].Actual != nil ||
		got.Observations[1].Surprise != nil || got.Observations[1].SurpriseDirection != nil ||
		got.Observations[1].Previous == nil || *got.Observations[1].Previous != "3.2%" {
		t.Errorf("output = %#v, want complete UTC event and observations in input order", got)
	}
	assertEconomicEventContextObservationMapping(t, got.Observations[0], latest, older, delta)
	assertEconomicEventContextSourceMapping(t, got.SourceRecords, context.SourceRecords)
	assertEconomicEventContextOutputJSON(t, got)
}

func TestNewEconomicEventContextOutputPreservesEmptyArrays(t *testing.T) {
	got := newEconomicEventContextOutput(intelligence.EventContext{})
	if got.Observations == nil || len(got.Observations) != 0 ||
		got.SourceRecords == nil || len(got.SourceRecords) != 0 {
		t.Fatalf("arrays = (%#v, %#v), want non-nil empty results", got.Observations, got.SourceRecords)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}
	if !strings.Contains(string(encoded), `"observations":[]`) ||
		!strings.Contains(string(encoded), `"source_records":[]`) {
		t.Errorf("output = %s, want non-nil empty JSON arrays", encoded)
	}
}

func assertEconomicEventContextObservationMapping(
	t *testing.T,
	got economicEventContextObservationOutput,
	latest intelligence.StoredObservation,
	older intelligence.StoredObservation,
	delta string,
) {
	t.Helper()
	if len(got.Revisions) != 2 || got.Revisions[0].ID != latest.ID ||
		got.Revisions[1].ID != older.ID || got.Revisions[1].SourceURL != older.SourceURL ||
		got.Revisions[1].Actual == nil || *got.Revisions[1].Actual != "3.10%" ||
		got.Revisions[1].ObservedAt != "2026-07-12T12:00:00Z" || len(got.Comparisons) != 1 ||
		got.Comparisons[0].NewerRevisionID != latest.ID ||
		got.Comparisons[0].OlderRevisionID != older.ID || len(got.Comparisons[0].Changes) != 3 {
		t.Fatalf("observation = %#v, want complete ordered revisions and comparison", got)
	}
	changes := got.Comparisons[0].Changes
	if changes[0].Field != intelligence.ObservationRevisionFieldPrevious ||
		changes[0].OldValue != nil || changes[0].NewValue == nil ||
		*changes[0].NewValue != "3.0%" || changes[0].Delta != nil ||
		changes[1].Field != intelligence.ObservationRevisionFieldActual ||
		changes[1].OldValue == nil || *changes[1].OldValue != "3.10%" ||
		changes[1].NewValue == nil || *changes[1].NewValue != "3.3%" ||
		changes[1].Delta == nil || *changes[1].Delta != delta ||
		changes[2].Field != intelligence.ObservationRevisionFieldSourceURL ||
		changes[2].OldValue == nil || *changes[2].OldValue != older.SourceURL ||
		changes[2].NewValue == nil || *changes[2].NewValue != latest.SourceURL ||
		changes[2].Delta != nil {
		t.Errorf("changes = %#v, want exact nullable values and copied numeric delta", changes)
	}
}

func assertEconomicEventContextSourceMapping(
	t *testing.T,
	got []economicEventSourceOutput,
	want []search.SimilarSourceRecord,
) {
	t.Helper()
	if len(got) != 2 {
		t.Fatalf("source records = %#v, want two records", got)
	}
	wantFirst := economicEventSourceOutput{
		ID:             want[0].SourceRecord.ID,
		Source:         "publisher",
		SourceItemID:   "item-" + want[0].SourceRecord.ID,
		OriginalURL:    "https://example.com/news/" + want[0].SourceRecord.ID,
		Title:          "Second",
		PublishedAt:    "2026-07-12T08:00:00Z",
		RetrievedAt:    "2026-07-12T08:01:00Z",
		CreatedAt:      "2026-07-12T08:02:00Z",
		UpdatedAt:      "2026-07-12T08:03:00Z",
		CreatedBy:      "rss-ingestion",
		UpdatedBy:      "rss-refresh",
		Provider:       "openai",
		Model:          "embedding-model",
		CosineDistance: 0.1,
	}
	if !reflect.DeepEqual(got[0], wantFirst) || got[1].ID != want[1].SourceRecord.ID {
		t.Errorf("source records = %#v, want complete UTC records in input order", got)
	}
}

func assertEconomicEventContextOutputJSON(t *testing.T, output economicEventContextOutput) {
	t.Helper()
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode output fields: %v", err)
	}
	if _, exists := fields["observations"]; !exists || len(fields) != 5 {
		t.Errorf("output keys = %v, want complete event context schema", reflect.ValueOf(fields).MapKeys())
	}
	text := string(encoded)
	for _, fragment := range []string{
		`"consensus":null`,
		`"actual":null`,
		`"surprise":null`,
		`"surprise_direction":null`,
		`"old_value":null`,
		`"delta":"+0.2%"`,
		`"delta":null`,
		`"revisions":[]`,
		`"comparisons":[]`,
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("output = %s, want JSON fragment %s", text, fragment)
		}
	}
}
