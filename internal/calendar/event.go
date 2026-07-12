// Package calendar models normalized scheduled economic events.
package calendar

import "time"

// Region identifies the economy associated with an event.
type Region string

const (
	RegionUnitedStates Region = "united_states"
	RegionEurozone     Region = "eurozone"
)

// MaxUpcomingEventsLimit bounds one upcoming-event retrieval.
const MaxUpcomingEventsLimit = 100

// MaxWatchlistEventCandidatesLimit bounds one watchlist event-candidate retrieval.
const MaxWatchlistEventCandidatesLimit = 100

// EventType classifies an economic event by the indicator or decision it represents.
type EventType string

const (
	EventTypeInflation            EventType = "inflation"
	EventTypeEmployment           EventType = "employment"
	EventTypeInterestRateDecision EventType = "interest_rate_decision"
	EventTypeGDP                  EventType = "gdp"
	EventTypePMI                  EventType = "pmi"
	EventTypeRetailSales          EventType = "retail_sales"
)

// Event is the normalized representation of one scheduled economic event.
type Event struct {
	Source          string
	ExternalEventID string
	Name            string
	Region          Region
	Type            EventType
	ScheduledAt     time.Time
	SourceURL       string
	RetrievedAt     time.Time
}

// StoredEvent is a normalized economic event with its persistence metadata.
type StoredEvent struct {
	ID string
	Event
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}
