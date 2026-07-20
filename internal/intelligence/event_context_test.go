package intelligence

import (
	"reflect"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/search"
)

func TestAssembleEventContextUsesExactEventNameAndPreservesOrderedCanonicalResults(t *testing.T) {
	location := time.FixedZone("CEST", 2*60*60)
	windowStart := time.Date(2026, time.July, 10, 9, 0, 0, 0, location)
	windowEnd := windowStart.Add(6 * time.Hour)
	event := storedEventFixture("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "  Consumer Price Index  ")
	results := []search.SimilarSourceRecord{
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000002", "Closer source", 0.1),
		similarSourceRecordFixture("00000000-0000-0000-0000-000000000001", "Farther source", 0.4),
	}
	observationResults := []StoredObservation{
		storedObservationFixture("00000000-0000-0000-0000-000000000002", event.ID, "latest", windowEnd.UTC()),
		storedObservationFixture("00000000-0000-0000-0000-000000000001", event.ID, "earlier", windowStart.UTC()),
	}
	compatibleConsensus := "3.2%"
	observationResults[0].Consensus = &compatibleConsensus
	observationResults[1].Consensus = nil
	observationResults[1].Actual = nil
	events := &economicEventReaderStub{event: event}
	observations := &observationReaderStub{results: observationResults}
	observationRevisionResults := [][]StoredObservation{
		{
			observationResults[0],
			storedObservationFixture(
				"00000000-0000-0000-0000-000000000003",
				event.ID,
				"latest",
				windowEnd.Add(-time.Hour).UTC(),
			),
		},
		{},
	}
	observationRevisions := &observationRevisionReaderStub{results: observationRevisionResults}
	embedder := &embedderStub{batch: search.EmbeddingBatch{
		Provider: " openai ",
		Model:    " embedding-model ",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{0.25, 0.5},
		}},
	}}
	sources := &similarSourceRecordReaderStub{results: results}
	query := EventContextQuery{
		EventID:                  "AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA",
		PublicationWindowStart:   windowStart,
		PublicationWindowEnd:     windowEnd,
		SourceRecordLimit:        17,
		ObservationLimit:         19,
		ObservationRevisionLimit: 23,
	}

	got, err := AssembleEventContext(
		t.Context(), events, observations, observationRevisions, embedder, sources, query,
	)
	if err != nil {
		t.Fatalf("AssembleEventContext() error = %v", err)
	}
	wantSurprise := "+0.1%"
	wantSurpriseDirection := SurpriseDirectionAboveConsensus
	wantActualChange := "+0.2%"
	want := EventContext{
		Event:                  event,
		PublicationWindowStart: windowStart.UTC(),
		PublicationWindowEnd:   windowEnd.UTC(),
		Observations: []EventContextObservation{
			{
				Latest:            observationResults[0],
				Surprise:          &wantSurprise,
				SurpriseDirection: &wantSurpriseDirection,
				ActualChange:      &wantActualChange,
				Revisions:         observationRevisionResults[0],
				Comparisons:       compareObservationRevisions(observationRevisionResults[0]),
			},
			{
				Latest:      observationResults[1],
				Revisions:   observationRevisionResults[1],
				Comparisons: []ObservationRevisionComparison{},
			},
		},
		SourceRecords: results,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AssembleEventContext() = %#v, want %#v", got, want)
	}
	if events.calls != 1 || events.id != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("economic event retrieval = (%d, %q), want normalized UUID", events.calls, events.id)
	}
	if observations.calls != 1 || observations.eventID != event.ID || observations.limit != 19 {
		t.Errorf(
			"observation retrieval = (%d, %q, %d), want canonical event UUID and limit",
			observations.calls,
			observations.eventID,
			observations.limit,
		)
	}
	wantRevisionCalls := []observationRevisionReaderCall{
		{eventID: event.ID, source: "official-statistics", sourceObservationID: "latest", limit: 23},
		{eventID: event.ID, source: "official-statistics", sourceObservationID: "earlier", limit: 23},
	}
	if !reflect.DeepEqual(observationRevisions.calls, wantRevisionCalls) {
		t.Errorf("observation revision retrieval = %#v, want %#v", observationRevisions.calls, wantRevisionCalls)
	}
	wantInputs := []search.EmbeddingInput{{SourceRecordID: "semantic-search-query", Text: event.Name}}
	if embedder.calls != 1 || !reflect.DeepEqual(embedder.inputs, wantInputs) {
		t.Errorf("embedding call = (%d, %#v), want exact persisted event name", embedder.calls, embedder.inputs)
	}
	if sources.calls != 1 || sources.provider != "openai" || sources.model != "embedding-model" ||
		!reflect.DeepEqual(sources.vector, []float32{0.25, 0.5}) || sources.limit != 17 {
		t.Errorf(
			"source retrieval = (%d, %q, %q, %#v, %d), want normalized provenance, vector, and limit",
			sources.calls,
			sources.provider,
			sources.model,
			sources.vector,
			sources.limit,
		)
	}
	if sources.filters.Source != nil || sources.filters.PublicationWindowStart == nil ||
		sources.filters.PublicationWindowEnd == nil ||
		*sources.filters.PublicationWindowStart != windowStart.UTC() ||
		*sources.filters.PublicationWindowEnd != windowEnd.UTC() {
		t.Errorf("source filters = %#v, want unfiltered source and normalized inclusive window", sources.filters)
	}
}

func TestAssembleEventContextPreservesNonNilEmptySourceResults(t *testing.T) {
	want := []search.SimilarSourceRecord{}
	got, err := AssembleEventContext(
		t.Context(),
		&economicEventReaderStub{event: storedEventFixture(validEventID, "Inflation")},
		&observationReaderStub{results: []StoredObservation{}},
		&observationRevisionReaderStub{},
		&embedderStub{batch: validEmbeddingBatch()},
		&similarSourceRecordReaderStub{results: want},
		validEventContextQuery(),
	)
	if err != nil {
		t.Fatalf("AssembleEventContext() error = %v", err)
	}
	if got.SourceRecords == nil || !reflect.DeepEqual(got.SourceRecords, want) {
		t.Errorf("AssembleEventContext().SourceRecords = %#v, want non-nil empty result", got.SourceRecords)
	}
}
