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
