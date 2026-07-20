package intelligencecmd

import "github.com/Yanis897349/atlas/internal/intelligence"

type economicEventContextObservationOutput struct {
	economicEventObservationOutput
	Surprise          *string                                    `json:"surprise"`
	SurpriseDirection *intelligence.SurpriseDirection            `json:"surprise_direction"`
	Revisions         []economicEventObservationOutput           `json:"revisions"`
	Comparisons       []economicEventObservationComparisonOutput `json:"comparisons"`
}

type economicEventObservationComparisonOutput struct {
	NewerRevisionID string                                 `json:"newer_revision_id"`
	OlderRevisionID string                                 `json:"older_revision_id"`
	Changes         []economicEventObservationChangeOutput `json:"changes"`
}

type economicEventObservationChangeOutput struct {
	Field    intelligence.ObservationRevisionField `json:"field"`
	OldValue *string                               `json:"old_value"`
	NewValue *string                               `json:"new_value"`
	Delta    *string                               `json:"delta"`
}

func newEconomicEventContextObservationOutput(
	observation intelligence.EventContextObservation,
) economicEventContextObservationOutput {
	result := economicEventContextObservationOutput{
		economicEventObservationOutput: newEconomicEventObservationOutput(observation.Latest),
		Surprise:                       observation.Surprise,
		SurpriseDirection:              observation.SurpriseDirection,
		Revisions: make(
			[]economicEventObservationOutput,
			0,
			len(observation.Revisions),
		),
		Comparisons: make(
			[]economicEventObservationComparisonOutput,
			0,
			len(observation.Comparisons),
		),
	}
	for _, revision := range observation.Revisions {
		result.Revisions = append(result.Revisions, newEconomicEventObservationOutput(revision))
	}
	for _, comparison := range observation.Comparisons {
		result.Comparisons = append(
			result.Comparisons,
			newEconomicEventObservationComparisonOutput(comparison),
		)
	}
	return result
}

func newEconomicEventObservationComparisonOutput(
	comparison intelligence.ObservationRevisionComparison,
) economicEventObservationComparisonOutput {
	result := economicEventObservationComparisonOutput{
		NewerRevisionID: comparison.NewerRevisionID,
		OlderRevisionID: comparison.OlderRevisionID,
		Changes: make(
			[]economicEventObservationChangeOutput,
			0,
			len(comparison.Changes),
		),
	}
	for _, change := range comparison.Changes {
		result.Changes = append(result.Changes, economicEventObservationChangeOutput{
			Field:    change.Field,
			OldValue: change.OldValue,
			NewValue: change.NewValue,
			Delta:    change.Delta,
		})
	}
	return result
}
