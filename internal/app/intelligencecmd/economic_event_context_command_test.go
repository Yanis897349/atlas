package intelligencecmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

func TestRunEconomicEventContextWritesCompleteOrderedContext(t *testing.T) {
	paris := time.FixedZone("Paris", 2*60*60)
	windowStart := time.Date(2026, time.July, 12, 10, 0, 0, 0, paris)
	windowEnd := windowStart.Add(4 * time.Hour)
	event := storedEventFixture("  Consumer Price Index  ", windowEnd)
	events := &economicEventReaderStub{event: event}
	observationResults := []intelligence.StoredObservation{
		{
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
		},
		{
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
		},
	}
	observations := &observationReaderStub{results: observationResults}
	olderRevision := observationResults[0]
	olderRevision.ID = "00000000-0000-0000-0000-000000000085"
	olderRevision.SourceURL = "https://example.com/releases/cpi-2026-07-initial"
	olderRevision.ObservedAt = windowEnd
	olderRevision.Actual = nil
	observationRevisions := &observationRevisionReaderStub{resultsByCall: [][]intelligence.StoredObservation{
		{observationResults[0], olderRevision},
		{},
	}}
	embedder := &embedderStub{batch: search.EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{1, 2},
		}},
	}}
	results := []search.SimilarSourceRecord{
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000002", "Second", windowStart, 0.1),
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000001", "First", windowStart.Add(time.Hour), 0.4),
	}
	sources := &similarSourceRecordReaderStub{results: results}
	stdout := &bytes.Buffer{}
	query := intelligence.EventContextQuery{
		EventID:                  strings.ToUpper(validEventID),
		PublicationWindowStart:   windowStart,
		PublicationWindowEnd:     windowEnd,
		SourceRecordLimit:        2,
		ObservationLimit:         7,
		ObservationRevisionLimit: 5,
	}

	if err := runEconomicEventContext(
		t.Context(), events, observations, observationRevisions, embedder, sources, stdout, query,
	); err != nil {
		t.Fatalf("runEconomicEventContext() error = %v", err)
	}
	wantRevisionCalls := []observationRevisionReaderInput{
		{eventID: validEventID, source: "official-statistics", sourceObservationID: "cpi-2026-07", limit: 5},
		{eventID: validEventID, source: "secondary-statistics", sourceObservationID: "cpi-secondary-2026-07", limit: 5},
	}
	if !reflect.DeepEqual(observationRevisions.calls, wantRevisionCalls) {
		t.Errorf("observation revision calls = %#v, want %#v", observationRevisions.calls, wantRevisionCalls)
	}
	if events.id != validEventID || observations.eventID != validEventID || observations.limit != 7 ||
		!reflect.DeepEqual(embedder.inputs, []search.EmbeddingInput{{
			SourceRecordID: "semantic-search-query", Text: event.Name,
		}}) || sources.provider != "openai" || sources.model != "embedding-model" ||
		!reflect.DeepEqual(sources.vector, []float32{1, 2}) || sources.limit != 2 ||
		sources.filters.PublicationWindowStart == nil ||
		*sources.filters.PublicationWindowStart != windowStart.UTC() ||
		sources.filters.PublicationWindowEnd == nil || *sources.filters.PublicationWindowEnd != windowEnd.UTC() {
		t.Errorf("orchestration = events %#v, embedder %#v, sources %#v", events, embedder, sources)
	}

	var got economicEventContextOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &encoded); err != nil {
		t.Fatalf("decode output keys: %v", err)
	}
	if _, exists := encoded["observations"]; !exists || len(encoded) != 5 {
		t.Errorf("output keys = %v, want event context schema with observations", reflect.ValueOf(encoded).MapKeys())
	}
	if got.Event.ID != validEventID || got.Event.SourceURL == "" || got.Event.CreatedBy != "calendar-ingestion" ||
		got.Event.UpdatedBy != "calendar-refresh" || got.Event.ScheduledAt != "2026-07-12T12:00:00Z" ||
		got.PublicationWindowStart != "2026-07-12T08:00:00Z" ||
		got.PublicationWindowEnd != "2026-07-12T12:00:00Z" || len(got.Observations) != 2 ||
		got.Observations[0].ID != observationResults[0].ID ||
		got.Observations[1].ID != observationResults[1].ID ||
		got.Observations[0].EconomicEventID != event.ID ||
		got.Observations[0].Source != "official-statistics" ||
		got.Observations[0].SourceObservationID != "cpi-2026-07" ||
		got.Observations[0].SourceURL != "https://example.com/releases/cpi-2026-07" ||
		got.Observations[0].ObservedAt != "2026-07-12T13:00:00Z" ||
		got.Observations[0].Consensus == nil || *got.Observations[0].Consensus != "3.1%" ||
		got.Observations[0].Previous == nil || *got.Observations[0].Previous != "3.0%" ||
		got.Observations[0].Actual == nil || *got.Observations[0].Actual != "3.3%" ||
		got.Observations[0].Surprise == nil || *got.Observations[0].Surprise != "+0.2%" ||
		got.Observations[0].SurpriseDirection == nil ||
		*got.Observations[0].SurpriseDirection != intelligence.SurpriseDirectionAboveConsensus ||
		got.Observations[0].CreatedAt != "2026-07-12T14:00:00Z" ||
		got.Observations[0].UpdatedAt != "2026-07-12T15:00:00Z" ||
		got.Observations[0].CreatedBy != "observation-ingestion" ||
		got.Observations[0].UpdatedBy != "observation-refresh" ||
		got.Observations[1].Consensus != nil || got.Observations[1].Actual != nil ||
		got.Observations[1].Surprise != nil || got.Observations[1].SurpriseDirection != nil ||
		got.Observations[1].Previous == nil || *got.Observations[1].Previous != "3.2%" ||
		len(got.Observations[0].Revisions) != 2 ||
		got.Observations[0].Revisions[0].ID != observationResults[0].ID ||
		got.Observations[0].Revisions[1].ID != olderRevision.ID ||
		got.Observations[0].Revisions[1].SourceURL != olderRevision.SourceURL ||
		got.Observations[0].Revisions[1].Actual != nil ||
		got.Observations[0].Revisions[1].ObservedAt != "2026-07-12T12:00:00Z" ||
		len(got.Observations[0].Comparisons) != 1 ||
		got.Observations[0].Comparisons[0].NewerRevisionID != observationResults[0].ID ||
		got.Observations[0].Comparisons[0].OlderRevisionID != olderRevision.ID ||
		len(got.Observations[0].Comparisons[0].Changes) != 2 ||
		got.Observations[0].Comparisons[0].Changes[0].Field != intelligence.ObservationRevisionFieldActual ||
		got.Observations[0].Comparisons[0].Changes[0].OldValue != nil ||
		got.Observations[0].Comparisons[0].Changes[0].NewValue == nil ||
		*got.Observations[0].Comparisons[0].Changes[0].NewValue != "3.3%" ||
		got.Observations[0].Comparisons[0].Changes[1].Field != intelligence.ObservationRevisionFieldSourceURL ||
		got.Observations[0].Comparisons[0].Changes[1].OldValue == nil ||
		*got.Observations[0].Comparisons[0].Changes[1].OldValue != olderRevision.SourceURL ||
		got.Observations[0].Comparisons[0].Changes[1].NewValue == nil ||
		*got.Observations[0].Comparisons[0].Changes[1].NewValue != observationResults[0].SourceURL ||
		got.Observations[1].Revisions == nil || len(got.Observations[1].Revisions) != 0 ||
		got.Observations[1].Comparisons == nil || len(got.Observations[1].Comparisons) != 0 ||
		len(got.SourceRecords) != 2 ||
		got.SourceRecords[0].ID != results[0].SourceRecord.ID ||
		got.SourceRecords[1].ID != results[1].SourceRecord.ID ||
		got.SourceRecords[0].PublishedAt != "2026-07-12T08:00:00Z" ||
		got.SourceRecords[0].CreatedBy != "rss-ingestion" || got.SourceRecords[0].UpdatedBy != "rss-refresh" ||
		got.SourceRecords[0].Provider != "openai" || got.SourceRecords[0].Model != "embedding-model" ||
		got.SourceRecords[0].CosineDistance != 0.1 {
		t.Errorf("output = %#v, want complete UTC event context in repository order", got)
	}
	if !strings.Contains(stdout.String(), `"consensus":null`) ||
		!strings.Contains(stdout.String(), `"actual":null`) ||
		!strings.Contains(stdout.String(), `"surprise":null`) ||
		!strings.Contains(stdout.String(), `"surprise_direction":null`) ||
		!strings.Contains(stdout.String(), `"old_value":null`) ||
		!strings.Contains(stdout.String(), `"delta":null`) ||
		!strings.Contains(stdout.String(), `"comparisons":[]`) {
		t.Errorf("output = %q, want nullable observation values encoded as null", stdout.String())
	}
}

func TestRunEconomicEventContextWritesNumericRevisionDeltas(t *testing.T) {
	event := storedEventFixture("Inflation", time.Now())
	older := intelligence.StoredObservation{
		ID: "00000000-0000-0000-0000-000000000084",
		Observation: intelligence.Observation{
			EconomicEventID:     event.ID,
			Source:              "official-statistics",
			SourceObservationID: "cpi-2026-07",
			SourceURL:           "https://example.com/releases/cpi-2026-07",
			ObservedAt:          time.Now().Add(-time.Hour),
			Actual:              observationValue("3.10%"),
		},
	}
	newer := older
	newer.ID = "00000000-0000-0000-0000-000000000085"
	newer.ObservedAt = time.Now()
	newer.Actual = observationValue("3.3%")
	stdout := &bytes.Buffer{}

	err := runEconomicEventContext(
		t.Context(),
		&economicEventReaderStub{event: event},
		&observationReaderStub{results: []intelligence.StoredObservation{newer}},
		&observationRevisionReaderStub{resultsByCall: [][]intelligence.StoredObservation{{newer, older}}},
		&embedderStub{batch: validEmbeddingBatch()},
		&similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}},
		stdout,
		validEventContextQuery(),
	)
	if err != nil {
		t.Fatalf("runEconomicEventContext() error = %v", err)
	}

	var output economicEventContextOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	changes := output.Observations[0].Comparisons[0].Changes
	if len(changes) != 1 || changes[0].Field != intelligence.ObservationRevisionFieldActual ||
		changes[0].OldValue == nil || *changes[0].OldValue != "3.10%" ||
		changes[0].NewValue == nil || *changes[0].NewValue != "3.3%" ||
		changes[0].Delta == nil || *changes[0].Delta != "+0.2%" {
		t.Errorf("changes = %#v, want exact raw values and +0.2%% delta", changes)
	}
	if !strings.Contains(stdout.String(), `"delta":"+0.2%"`) {
		t.Errorf("output = %q, want delta JSON field", stdout.String())
	}
}

func TestRunEconomicEventContextWritesEmptyArrays(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runEconomicEventContext(
		t.Context(),
		&economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())},
		&observationReaderStub{results: []intelligence.StoredObservation{}},
		&observationRevisionReaderStub{},
		&embedderStub{batch: validEmbeddingBatch()},
		&similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}},
		stdout,
		validEventContextQuery(),
	)
	if err != nil {
		t.Fatalf("runEconomicEventContext() error = %v", err)
	}
	var output struct {
		Observations  []economicEventContextObservationOutput `json:"observations"`
		SourceRecords []economicEventSourceOutput             `json:"source_records"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output.Observations == nil || len(output.Observations) != 0 ||
		output.SourceRecords == nil || len(output.SourceRecords) != 0 ||
		!strings.Contains(stdout.String(), `"observations":[]`) ||
		!strings.Contains(stdout.String(), `"source_records":[]`) {
		t.Errorf("arrays = (%#v, %#v) (%q), want non-nil empty JSON arrays", output.Observations, output.SourceRecords, stdout.String())
	}
}
