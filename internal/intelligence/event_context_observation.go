package intelligence

import (
	"context"
	"fmt"
)

// EventContextObservation contains one latest observation and its bounded immutable revisions.
type EventContextObservation struct {
	Latest            StoredObservation
	Surprise          *string
	SurpriseDirection *SurpriseDirection
	ActualChange      *string
	Revisions         []StoredObservation
	Comparisons       []ObservationRevisionComparison
}

func assembleEventContextObservations(
	ctx context.Context,
	revisionReader ObservationRevisionReader,
	eventID string,
	observations []StoredObservation,
	revisionLimit int,
) ([]EventContextObservation, error) {
	histories := make([]EventContextObservation, 0, len(observations))
	for _, observation := range observations {
		revisions, err := revisionReader.ObservationRevisions(
			ctx,
			eventID,
			observation.Source,
			observation.SourceObservationID,
			revisionLimit,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"retrieve economic event observation revisions for source %q identity %q: %w",
				observation.Source,
				observation.SourceObservationID,
				err,
			)
		}
		if len(revisions) > 0 && revisions[0].ID != observation.ID {
			return nil, fmt.Errorf(
				"validate economic event observation revisions for source %q identity %q: latest revision %q does not match selected observation %q",
				observation.Source,
				observation.SourceObservationID,
				revisions[0].ID,
				observation.ID,
			)
		}

		surprise, surpriseDirection := observationNumericSurprise(observation.Consensus, observation.Actual)
		histories = append(histories, EventContextObservation{
			Latest:            observation,
			Surprise:          surprise,
			SurpriseDirection: surpriseDirection,
			ActualChange:      observationNumericActualChange(observation.Previous, observation.Actual),
			Revisions:         revisions,
			Comparisons:       compareObservationRevisions(revisions),
		})
	}
	return histories, nil
}
