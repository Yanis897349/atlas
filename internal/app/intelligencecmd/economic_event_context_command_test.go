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

func TestRunEconomicEventContextAssemblesAndWrites(t *testing.T) {
	paris := time.FixedZone("Paris", 2*60*60)
	windowStart := time.Date(2026, time.July, 12, 10, 0, 0, 0, paris)
	windowEnd := windowStart.Add(4 * time.Hour)
	event := storedEventFixture("  Consumer Price Index  ", windowEnd)
	observation := intelligence.StoredObservation{
		ID: "00000000-0000-0000-0000-000000000087",
		Observation: intelligence.Observation{
			EconomicEventID:     event.ID,
			Source:              "official-statistics",
			SourceObservationID: "cpi-2026-07",
			SourceURL:           "https://example.com/releases/cpi-2026-07",
			ObservedAt:          windowEnd.Add(time.Hour),
		},
	}
	match := similarSourceRecordFixture(
		"00000000-0000-0000-0000-000000000002",
		"Second",
		windowStart,
		0.1,
	)
	events := &economicEventReaderStub{event: event}
	observations := &observationReaderStub{results: []intelligence.StoredObservation{observation}}
	revisions := &observationRevisionReaderStub{
		resultsByCall: [][]intelligence.StoredObservation{{observation}},
	}
	embedder := &embedderStub{batch: search.EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{1, 2},
		}},
	}}
	sources := &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{match}}
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
		t.Context(), events, observations, revisions, embedder, sources, stdout, query,
	); err != nil {
		t.Fatalf("runEconomicEventContext() error = %v", err)
	}
	if events.id != validEventID || observations.eventID != validEventID || observations.limit != 7 ||
		!reflect.DeepEqual(revisions.calls, []observationRevisionReaderInput{{
			eventID: validEventID, source: "official-statistics",
			sourceObservationID: "cpi-2026-07", limit: 5,
		}}) ||
		!reflect.DeepEqual(embedder.inputs, []search.EmbeddingInput{{
			SourceRecordID: "semantic-search-query", Text: event.Name,
		}}) || sources.provider != "openai" || sources.model != "embedding-model" ||
		!reflect.DeepEqual(sources.vector, []float32{1, 2}) || sources.limit != 2 ||
		sources.filters.PublicationWindowStart == nil ||
		*sources.filters.PublicationWindowStart != windowStart.UTC() ||
		sources.filters.PublicationWindowEnd == nil ||
		*sources.filters.PublicationWindowEnd != windowEnd.UTC() {
		t.Errorf("orchestration = events %#v, revisions %#v, embedder %#v, sources %#v", events, revisions, embedder, sources)
	}

	var output struct {
		Event struct {
			ID string `json:"id"`
		} `json:"event"`
		Observations []struct {
			ID string `json:"id"`
		} `json:"observations"`
		SourceRecords []struct {
			ID string `json:"id"`
		} `json:"source_records"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output.Event.ID != event.ID || len(output.Observations) != 1 ||
		output.Observations[0].ID != observation.ID || len(output.SourceRecords) != 1 ||
		output.SourceRecords[0].ID != match.SourceRecord.ID {
		t.Errorf("output = %#v, want assembled event, observation, and source record", output)
	}
}
