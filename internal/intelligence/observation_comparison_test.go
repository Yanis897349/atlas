package intelligence

import (
	"reflect"
	"testing"
)

func TestCompareObservationRevisionsPreservesOrderedExactChanges(t *testing.T) {
	oldest := comparisonObservationFixture(
		"00000000-0000-0000-0000-000000000001",
		"https://example.com/releases/initial",
		observationComparisonValue("3.00%"),
		nil,
		observationComparisonValue("3.1%"),
	)
	middle := comparisonObservationFixture(
		"00000000-0000-0000-0000-000000000002",
		"https://example.com/releases/corrected",
		nil,
		observationComparisonValue("2.900%"),
		observationComparisonValue("3.1%"),
	)
	newest := comparisonObservationFixture(
		"00000000-0000-0000-0000-000000000003",
		middle.SourceURL,
		observationComparisonValue("3.200%"),
		observationComparisonValue("2.900%"),
		nil,
	)

	got := compareObservationRevisions([]StoredObservation{newest, middle, oldest})
	want := []ObservationRevisionComparison{
		{
			NewerRevisionID: newest.ID,
			OlderRevisionID: middle.ID,
			Changes: []ObservationRevisionChange{
				{
					Field:    ObservationRevisionFieldConsensus,
					OldValue: nil,
					NewValue: newest.Consensus,
				},
				{
					Field:    ObservationRevisionFieldActual,
					OldValue: middle.Actual,
					NewValue: nil,
				},
			},
		},
		{
			NewerRevisionID: middle.ID,
			OlderRevisionID: oldest.ID,
			Changes: []ObservationRevisionChange{
				{
					Field:    ObservationRevisionFieldConsensus,
					OldValue: oldest.Consensus,
					NewValue: nil,
				},
				{
					Field:    ObservationRevisionFieldPrevious,
					OldValue: nil,
					NewValue: middle.Previous,
				},
				{
					Field:    ObservationRevisionFieldSourceURL,
					OldValue: &oldest.SourceURL,
					NewValue: &middle.SourceURL,
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compareObservationRevisions() = %#v, want %#v", got, want)
	}
}

func TestCompareObservationRevisionsPreservesEmptyResults(t *testing.T) {
	unchangedOlder := comparisonObservationFixture(
		"00000000-0000-0000-0000-000000000001",
		"https://example.com/releases/unchanged",
		observationComparisonValue("3.0%"),
		observationComparisonValue("2.9%"),
		observationComparisonValue("3.1%"),
	)
	unchangedNewer := unchangedOlder
	unchangedNewer.ID = "00000000-0000-0000-0000-000000000002"

	tests := []struct {
		name      string
		revisions []StoredObservation
		wantCount int
	}{
		{name: "no revisions", revisions: nil, wantCount: 0},
		{name: "one revision", revisions: []StoredObservation{unchangedNewer}, wantCount: 0},
		{name: "unchanged adjacent revisions", revisions: []StoredObservation{unchangedNewer, unchangedOlder}, wantCount: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := compareObservationRevisions(test.revisions)
			if got == nil || len(got) != test.wantCount {
				t.Fatalf("compareObservationRevisions() = %#v, want non-nil result of length %d", got, test.wantCount)
			}
			if test.wantCount == 1 && (got[0].Changes == nil || len(got[0].Changes) != 0) {
				t.Errorf("compareObservationRevisions()[0].Changes = %#v, want non-nil empty changes", got[0].Changes)
			}
		})
	}
}

func TestCompareObservationRevisionsCalculatesCompatibleNumericDeltas(t *testing.T) {
	older := comparisonObservationFixture(
		"00000000-0000-0000-0000-000000000001",
		"https://example.com/releases/initial",
		observationComparisonValue("3.00%"),
		observationComparisonValue("+100"),
		observationComparisonValue("1.0"),
	)
	newer := comparisonObservationFixture(
		"00000000-0000-0000-0000-000000000002",
		"https://example.com/releases/corrected",
		observationComparisonValue("3.200%"),
		observationComparisonValue("+50"),
		observationComparisonValue("1.00"),
	)

	got := compareObservationRevisions([]StoredObservation{newer, older})
	if len(got) != 1 || len(got[0].Changes) != 4 {
		t.Fatalf("compareObservationRevisions() = %#v, want four ordered changes", got)
	}
	wantFields := []ObservationRevisionField{
		ObservationRevisionFieldConsensus,
		ObservationRevisionFieldPrevious,
		ObservationRevisionFieldActual,
		ObservationRevisionFieldSourceURL,
	}
	wantDeltas := []*string{
		observationComparisonValue("+0.2%"),
		observationComparisonValue("-50"),
		observationComparisonValue("0"),
		nil,
	}
	for index, change := range got[0].Changes {
		if change.Field != wantFields[index] || !reflect.DeepEqual(change.Delta, wantDeltas[index]) {
			t.Errorf("changes[%d] = %#v, want field %q delta %#v", index, change, wantFields[index], wantDeltas[index])
		}
	}
}

func TestCompareObservationRevisionsLeavesUnavailableDeltasNil(t *testing.T) {
	older := comparisonObservationFixture(
		"00000000-0000-0000-0000-000000000001",
		"https://example.com/releases/unchanged",
		observationComparisonValue("3.0%"),
		observationComparisonValue("147,000"),
		observationComparisonValue("1K"),
	)
	newer := older
	newer.ID = "00000000-0000-0000-0000-000000000002"
	newer.Consensus = observationComparisonValue("3.2")
	newer.Previous = observationComparisonValue("148,000")
	newer.Actual = nil

	got := compareObservationRevisions([]StoredObservation{newer, older})
	if len(got) != 1 || len(got[0].Changes) != 3 {
		t.Fatalf("compareObservationRevisions() = %#v, want three changes", got)
	}
	for _, change := range got[0].Changes {
		if change.Delta != nil {
			t.Errorf("change = %#v, want nil delta", change)
		}
	}
}

func comparisonObservationFixture(
	id string,
	sourceURL string,
	consensus *string,
	previous *string,
	actual *string,
) StoredObservation {
	return StoredObservation{
		ID: id,
		Observation: Observation{
			SourceURL: sourceURL,
			Consensus: consensus,
			Previous:  previous,
			Actual:    actual,
		},
	}
}

func observationComparisonValue(value string) *string {
	return &value
}
