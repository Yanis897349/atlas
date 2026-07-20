package intelligence

// ObservationRevisionField identifies one comparable observation field.
type ObservationRevisionField string

const (
	ObservationRevisionFieldConsensus ObservationRevisionField = "consensus"
	ObservationRevisionFieldPrevious  ObservationRevisionField = "previous"
	ObservationRevisionFieldActual    ObservationRevisionField = "actual"
	ObservationRevisionFieldSourceURL ObservationRevisionField = "source_url"
)

// ObservationRevisionChange preserves one exact field change between adjacent revisions.
type ObservationRevisionChange struct {
	Field    ObservationRevisionField
	OldValue *string
	NewValue *string
	Delta    *string
}

// ObservationRevisionComparison describes changes from one older revision to its adjacent newer revision.
type ObservationRevisionComparison struct {
	NewerRevisionID string
	OlderRevisionID string
	Changes         []ObservationRevisionChange
}

func compareObservationRevisions(revisions []StoredObservation) []ObservationRevisionComparison {
	comparisons := make([]ObservationRevisionComparison, 0, max(len(revisions)-1, 0))
	for index := 0; index+1 < len(revisions); index++ {
		newer := revisions[index]
		older := revisions[index+1]
		changes := make([]ObservationRevisionChange, 0, 4)
		changes = appendObservationRevisionChange(
			changes,
			ObservationRevisionFieldConsensus,
			older.Consensus,
			newer.Consensus,
		)
		changes = appendObservationRevisionChange(
			changes,
			ObservationRevisionFieldPrevious,
			older.Previous,
			newer.Previous,
		)
		changes = appendObservationRevisionChange(
			changes,
			ObservationRevisionFieldActual,
			older.Actual,
			newer.Actual,
		)
		changes = appendObservationRevisionChange(
			changes,
			ObservationRevisionFieldSourceURL,
			&older.SourceURL,
			&newer.SourceURL,
		)
		comparisons = append(comparisons, ObservationRevisionComparison{
			NewerRevisionID: newer.ID,
			OlderRevisionID: older.ID,
			Changes:         changes,
		})
	}
	return comparisons
}

func appendObservationRevisionChange(
	changes []ObservationRevisionChange,
	field ObservationRevisionField,
	oldValue *string,
	newValue *string,
) []ObservationRevisionChange {
	if equalObservationRevisionValues(oldValue, newValue) {
		return changes
	}
	var delta *string
	switch field {
	case ObservationRevisionFieldConsensus,
		ObservationRevisionFieldPrevious,
		ObservationRevisionFieldActual:
		if oldValue != nil && newValue != nil {
			delta, _ = observationNumericDelta(*oldValue, *newValue)
		}
	}
	return append(changes, ObservationRevisionChange{
		Field:    field,
		OldValue: oldValue,
		NewValue: newValue,
		Delta:    delta,
	})
}

func equalObservationRevisionValues(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
