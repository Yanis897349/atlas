package app

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/intelligence"
)

func assertEconomicEventContextIntegrationOutput(
	t *testing.T,
	output economicEventContextIntegrationOutput,
	want economicEventContextIntegrationWant,
) {
	t.Helper()
	var outputFields map[string]json.RawMessage
	if err := json.Unmarshal(want.rawOutput, &outputFields); err != nil {
		t.Fatalf("decode command output fields: %v", err)
	}
	if _, exists := outputFields["observations"]; !exists || len(outputFields) != 5 {
		t.Errorf("command output fields = %v, want schema with observations", outputFields)
	}
	if output.Event.ID != want.event.ID || output.Event.Source != want.event.Source ||
		output.Event.ExternalEventID != want.event.ExternalEventID || output.Event.Name != want.event.Name ||
		output.Event.Region != want.event.Region || output.Event.EventType != want.event.Type ||
		output.Event.SourceURL != want.event.SourceURL || output.Event.ScheduledAt == "" ||
		output.Event.RetrievedAt == "" || output.Event.CreatedAt == "" || output.Event.UpdatedAt == "" ||
		output.Event.CreatedBy != "calendar-ingestion" || output.Event.UpdatedBy != "calendar-ingestion" {
		t.Errorf("event output = %#v, want complete canonical event", output.Event)
	}
	if output.PublicationWindowStart != want.windowStart.Format(time.RFC3339Nano) ||
		output.PublicationWindowEnd != want.windowEnd.Format(time.RFC3339Nano) {
		t.Errorf(
			"publication window = (%q, %q), want normalized inclusive bounds",
			output.PublicationWindowStart,
			output.PublicationWindowEnd,
		)
	}
	assertEconomicEventContextObservations(t, output, want)
	assertEconomicEventContextSourceRecords(t, output, want.sourceRecords)
}

func assertEconomicEventContextObservations(
	t *testing.T,
	output economicEventContextIntegrationOutput,
	want economicEventContextIntegrationWant,
) {
	t.Helper()
	wantObservationIDs := []string{
		want.observations["cpi-latest-2026-07"].ID,
		want.observations["cpi-2026-07"].ID,
		want.observations["cpi-inline-2026-07"].ID,
		want.observations["cpi-below-2026-07"].ID,
		want.observations["cpi-oldest-2026-07"].ID,
	}
	if len(output.Observations) != len(wantObservationIDs) {
		t.Fatalf("observations = %#v, want five bounded observations", output.Observations)
	}
	for index, wantID := range wantObservationIDs {
		got := output.Observations[index]
		stored := want.observations[got.SourceObservationID]
		if got.ID != wantID || got.EconomicEventID != want.event.ID || got.Source == "" ||
			got.SourceObservationID == "" || got.SourceURL == "" || got.ObservedAt == "" ||
			got.CreatedAt == "" || got.UpdatedAt == "" || got.CreatedBy != stored.CreatedBy ||
			got.UpdatedBy != stored.UpdatedBy {
			t.Errorf("observations[%d] = %#v, want complete canonical observation %q", index, got, wantID)
		}
	}
	for _, observation := range output.Observations {
		if observation.ID == want.observations["cpi-excluded-2026-07"].ID {
			t.Errorf("observations included %q beyond latest-observation limit", observation.ID)
		}
	}
	assertEconomicEventContextRevisions(t, output, want)
	assertEconomicEventContextComparisons(t, output, want)
	assertEconomicEventContextDerivedValues(t, output, want)
}

func assertEconomicEventContextRevisions(
	t *testing.T,
	output economicEventContextIntegrationOutput,
	want economicEventContextIntegrationWant,
) {
	t.Helper()
	if len(output.Observations[0].Revisions) != 2 ||
		output.Observations[0].Revisions[0].ID != want.latestRevision.ID ||
		output.Observations[0].Revisions[1].ID != want.latestInitial.ID ||
		output.Observations[0].Revisions[0].SourceURL != want.latestRevision.SourceURL ||
		output.Observations[0].Revisions[0].CreatedBy != "latest-observation-correction" ||
		output.Observations[0].Revisions[0].Previous == nil ||
		*output.Observations[0].Revisions[0].Previous != want.previous ||
		output.Observations[0].Revisions[0].Consensus == nil ||
		*output.Observations[0].Revisions[0].Consensus != want.consensus ||
		output.Observations[0].Revisions[0].Actual == nil ||
		*output.Observations[0].Revisions[0].Actual != want.actual {
		t.Errorf("latest identity revisions = %#v, want complete newest-first bounded history", output.Observations[0].Revisions)
	}
	if len(output.Observations[1].Revisions) != 2 ||
		output.Observations[1].Revisions[0].ID != want.officialLatest.ID ||
		output.Observations[1].Revisions[1].ID != want.officialCitation.ID ||
		output.Observations[1].Revisions[0].Consensus != nil ||
		output.Observations[1].Revisions[0].Actual == nil ||
		*output.Observations[1].Revisions[0].Actual != want.revisedActual ||
		output.Observations[1].Revisions[0].SourceURL != want.officialLatest.SourceURL ||
		output.Observations[1].Revisions[0].CreatedAt == "" ||
		output.Observations[1].Revisions[0].UpdatedAt == "" ||
		output.Observations[1].Revisions[0].CreatedBy != "observation-value-correction" ||
		output.Observations[1].Revisions[1].CreatedBy != "observation-citation-correction" {
		t.Errorf("official revisions = %#v, want complete exact newest-first bounded history", output.Observations[1].Revisions)
	}
	for _, revision := range output.Observations[1].Revisions {
		if revision.ID == want.officialInitial.ID {
			t.Errorf("official revisions included %q beyond per-identity limit", want.officialInitial.ID)
		}
	}
}

func assertEconomicEventContextComparisons(
	t *testing.T,
	output economicEventContextIntegrationOutput,
	want economicEventContextIntegrationWant,
) {
	t.Helper()
	if len(output.Observations[0].Comparisons) != 1 ||
		output.Observations[0].Comparisons[0].NewerRevisionID != want.latestRevision.ID ||
		output.Observations[0].Comparisons[0].OlderRevisionID != want.latestInitial.ID ||
		len(output.Observations[0].Comparisons[0].Changes) != 1 ||
		output.Observations[0].Comparisons[0].Changes[0].Field != "source_url" ||
		output.Observations[0].Comparisons[0].Changes[0].OldValue == nil ||
		*output.Observations[0].Comparisons[0].Changes[0].OldValue != want.latestInitial.SourceURL ||
		output.Observations[0].Comparisons[0].Changes[0].NewValue == nil ||
		*output.Observations[0].Comparisons[0].Changes[0].NewValue != want.latestRevision.SourceURL ||
		output.Observations[0].Comparisons[0].Changes[0].Delta != nil {
		t.Errorf(
			"latest identity comparisons = %#v, want exact adjacent citation comparison",
			output.Observations[0].Comparisons,
		)
	}
	if len(output.Observations[1].Comparisons) != 1 ||
		output.Observations[1].Comparisons[0].NewerRevisionID != want.officialLatest.ID ||
		output.Observations[1].Comparisons[0].OlderRevisionID != want.officialCitation.ID ||
		len(output.Observations[1].Comparisons[0].Changes) != 2 ||
		output.Observations[1].Comparisons[0].Changes[0].Field != "consensus" ||
		output.Observations[1].Comparisons[0].Changes[0].OldValue == nil ||
		*output.Observations[1].Comparisons[0].Changes[0].OldValue != want.consensus ||
		output.Observations[1].Comparisons[0].Changes[0].NewValue != nil ||
		output.Observations[1].Comparisons[0].Changes[0].Delta != nil ||
		output.Observations[1].Comparisons[0].Changes[1].Field != "actual" ||
		output.Observations[1].Comparisons[0].Changes[1].OldValue == nil ||
		*output.Observations[1].Comparisons[0].Changes[1].OldValue != want.actual ||
		output.Observations[1].Comparisons[0].Changes[1].NewValue == nil ||
		*output.Observations[1].Comparisons[0].Changes[1].NewValue != want.revisedActual ||
		output.Observations[1].Comparisons[0].Changes[1].Delta == nil ||
		*output.Observations[1].Comparisons[0].Changes[1].Delta != "+0.2%" {
		t.Errorf(
			"official comparisons = %#v, want exact adjacent nullable value comparison",
			output.Observations[1].Comparisons,
		)
	}
}

func assertEconomicEventContextDerivedValues(
	t *testing.T,
	output economicEventContextIntegrationOutput,
	want economicEventContextIntegrationWant,
) {
	t.Helper()
	if output.Observations[0].Consensus == nil || *output.Observations[0].Consensus != want.consensus ||
		output.Observations[0].Actual == nil || *output.Observations[0].Actual != want.actual ||
		output.Observations[0].Surprise == nil || *output.Observations[0].Surprise != "+0.2%" ||
		output.Observations[0].SurpriseDirection == nil ||
		*output.Observations[0].SurpriseDirection != intelligence.SurpriseDirectionAboveConsensus ||
		output.Observations[0].ActualChange == nil || *output.Observations[0].ActualChange != "+0.3%" ||
		output.Observations[0].Previous == nil || *output.Observations[0].Previous != want.previous ||
		output.Observations[1].Consensus != nil ||
		output.Observations[1].Previous == nil || *output.Observations[1].Previous != want.previous ||
		output.Observations[1].Actual == nil || *output.Observations[1].Actual != want.revisedActual ||
		output.Observations[1].Surprise != nil || output.Observations[1].SurpriseDirection != nil ||
		output.Observations[1].ActualChange == nil || *output.Observations[1].ActualChange != "+0.5%" ||
		output.Observations[2].Surprise == nil || *output.Observations[2].Surprise != "0%" ||
		output.Observations[2].SurpriseDirection == nil ||
		*output.Observations[2].SurpriseDirection != intelligence.SurpriseDirectionInLine ||
		output.Observations[3].Surprise == nil || *output.Observations[3].Surprise != "-0.1%" ||
		output.Observations[3].SurpriseDirection == nil ||
		*output.Observations[3].SurpriseDirection != intelligence.SurpriseDirectionBelowConsensus ||
		output.Observations[2].ActualChange != nil || output.Observations[3].ActualChange != nil ||
		output.Observations[4].Surprise != nil || output.Observations[4].SurpriseDirection != nil ||
		output.Observations[4].ActualChange != nil ||
		len(output.Observations[0].Revisions) == 0 ||
		output.Observations[0].Revisions[0].ActualChange != nil ||
		len(output.Observations[1].Revisions) == 0 ||
		output.Observations[1].Revisions[0].ActualChange != nil ||
		!bytes.Contains(want.rawOutput, []byte(`"actual_change":null`)) ||
		!bytes.Contains(want.rawOutput, []byte(`"surprise_direction":null`)) {
		t.Errorf("observation values = %#v, want exact nullable values", output.Observations)
	}
}

func assertEconomicEventContextSourceRecords(
	t *testing.T,
	output economicEventContextIntegrationOutput,
	records map[string]ingestion.StoredSourceRecord,
) {
	t.Helper()
	exact := []ingestion.StoredSourceRecord{records["start"], records["middle-a"]}
	sort.Slice(exact, func(left, right int) bool { return exact[left].ID < exact[right].ID })
	wantIDs := []string{exact[0].ID, exact[1].ID, records["middle-b"].ID, records["end"].ID}
	if len(output.SourceRecords) != len(wantIDs) {
		t.Fatalf("source records = %#v, want four inclusive matching records", output.SourceRecords)
	}
	for index, wantID := range wantIDs {
		got := output.SourceRecords[index]
		if got.ID != wantID || got.Source != "test-publisher" || got.SourceItemID == "" ||
			got.OriginalURL == "" || got.Title == "" || got.PublishedAt == "" || got.RetrievedAt == "" ||
			got.CreatedAt == "" || got.UpdatedAt == "" || got.CreatedBy != "rss-ingestion" ||
			got.UpdatedBy != "rss-ingestion" || got.Provider != "openai" || got.Model != "embedding-model" {
			t.Errorf("source_records[%d] = %#v, want complete canonical match %q", index, got, wantID)
		}
	}
	if output.SourceRecords[0].CosineDistance != 0 || output.SourceRecords[1].CosineDistance != 0 ||
		output.SourceRecords[2].CosineDistance <= 0 || output.SourceRecords[2].CosineDistance >= 1 ||
		output.SourceRecords[3].CosineDistance != 1 {
		t.Errorf("distances = [%v %v %v %v], want ordered [0 0 between-0-and-1 1]",
			output.SourceRecords[0].CosineDistance,
			output.SourceRecords[1].CosineDistance,
			output.SourceRecords[2].CosineDistance,
			output.SourceRecords[3].CosineDistance,
		)
	}
}
