package intelligencecmd

import (
	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/intelligence"
)

type economicEventObservationOutput struct {
	ID                  string  `json:"id"`
	EconomicEventID     string  `json:"economic_event_id"`
	Source              string  `json:"source"`
	SourceObservationID string  `json:"source_observation_id"`
	SourceURL           string  `json:"source_url"`
	ObservedAt          string  `json:"observed_at"`
	Consensus           *string `json:"consensus"`
	Previous            *string `json:"previous"`
	Actual              *string `json:"actual"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	CreatedBy           string  `json:"created_by"`
	UpdatedBy           string  `json:"updated_by"`
}

func newEconomicEventObservationOutput(
	observation intelligence.StoredObservation,
) economicEventObservationOutput {
	return economicEventObservationOutput{
		ID:                  observation.ID,
		EconomicEventID:     observation.EconomicEventID,
		Source:              observation.Source,
		SourceObservationID: observation.SourceObservationID,
		SourceURL:           observation.SourceURL,
		ObservedAt:          output.FormatTime(observation.ObservedAt),
		Consensus:           observation.Consensus,
		Previous:            observation.Previous,
		Actual:              observation.Actual,
		CreatedAt:           output.FormatTime(observation.CreatedAt),
		UpdatedAt:           output.FormatTime(observation.UpdatedAt),
		CreatedBy:           observation.CreatedBy,
		UpdatedBy:           observation.UpdatedBy,
	}
}
