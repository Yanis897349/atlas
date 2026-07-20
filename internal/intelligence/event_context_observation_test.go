package intelligence

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/search"
)

func TestAssembleEventContextPreservesNonNilEmptyObservationResults(t *testing.T) {
	want := []EventContextObservation{}
	got, err := AssembleEventContext(
		t.Context(),
		&economicEventReaderStub{event: storedEventFixture(validEventID, "Inflation")},
		&observationReaderStub{results: []StoredObservation{}},
		&observationRevisionReaderStub{},
		&embedderStub{batch: validEmbeddingBatch()},
		&similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}},
		validEventContextQuery(),
	)
	if err != nil {
		t.Fatalf("AssembleEventContext() error = %v", err)
	}
	if got.Observations == nil || !reflect.DeepEqual(got.Observations, want) {
		t.Errorf("AssembleEventContext().Observations = %#v, want non-nil empty result", got.Observations)
	}
}

func TestAssembleEventContextRejectsHistoryNewerThanSelectedObservation(t *testing.T) {
	selected := storedObservationFixture(
		"00000000-0000-0000-0000-000000000010",
		validEventID,
		"concurrent-revision",
		time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC),
	)
	newer := selected
	newer.ID = "00000000-0000-0000-0000-000000000011"
	newer.ObservedAt = selected.ObservedAt.Add(time.Minute)
	embedder := &embedderStub{batch: validEmbeddingBatch()}
	sources := &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}}

	got, err := AssembleEventContext(
		t.Context(),
		&economicEventReaderStub{event: storedEventFixture(validEventID, "Inflation")},
		&observationReaderStub{results: []StoredObservation{selected}},
		&observationRevisionReaderStub{results: [][]StoredObservation{{newer, selected}}},
		embedder,
		sources,
		validEventContextQuery(),
	)
	if err == nil || !strings.Contains(err.Error(), "validate economic event observation revisions") ||
		!strings.Contains(err.Error(), newer.ID) || !strings.Contains(err.Error(), selected.ID) {
		t.Fatalf("AssembleEventContext() error = %v, want contextual inconsistent history failure", err)
	}
	if !reflect.DeepEqual(got, EventContext{}) {
		t.Errorf("AssembleEventContext() = %#v, want zero context", got)
	}
	if embedder.calls != 0 || sources.calls != 0 {
		t.Errorf("downstream calls after inconsistent history = (%d, %d), want none", embedder.calls, sources.calls)
	}
}
