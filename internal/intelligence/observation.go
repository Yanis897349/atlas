package intelligence

import (
	"context"
	"time"
)

// MaxEventObservationsLimit bounds one economic-event observation retrieval.
const MaxEventObservationsLimit = 100

// Observation is one source snapshot of values associated with a canonical economic event.
type Observation struct {
	EconomicEventID     string
	Source              string
	SourceObservationID string
	SourceURL           string
	ObservedAt          time.Time
	Consensus           *string
	Previous            *string
	Actual              *string
}

// StoredObservation is an observation with persistence identity and audit metadata.
type StoredObservation struct {
	ID string
	Observation
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

// ObservationPersistence stores the latest snapshot for one source observation identity.
type ObservationPersistence interface {
	UpsertObservation(context.Context, Observation, string) (StoredObservation, error)
}

// ObservationReader retrieves observations associated with one canonical economic event.
type ObservationReader interface {
	EventObservations(context.Context, string, int) ([]StoredObservation, error)
}
