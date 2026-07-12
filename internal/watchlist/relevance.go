package watchlist

import (
	"errors"
	"fmt"

	"github.com/Yanis897349/atlas/internal/calendar"
)

var (
	// ErrUnsupportedInstrument indicates that no deterministic relevance rules exist for an instrument.
	ErrUnsupportedInstrument = errors.New("unsupported instrument")
	// ErrUnsupportedRegion indicates that no deterministic relevance rules exist for an event region.
	ErrUnsupportedRegion = errors.New("unsupported event region")
	// ErrUnsupportedEventType indicates that no deterministic relevance rules exist for an event type.
	ErrUnsupportedEventType = errors.New("unsupported event type")
)

// EventRelevance is the deterministic relevance classification of one canonical event for an instrument.
type EventRelevance struct {
	Symbol   string
	Event    calendar.StoredEvent
	Relevant bool
}

// ClassifyEventRelevance applies the supported deterministic instrument-to-region rules.
func ClassifyEventRelevance(symbol string, event calendar.StoredEvent) (EventRelevance, error) {
	canonicalSymbol := NormalizeInstrumentSymbol(symbol)
	if !supportedRelevanceInstrument(canonicalSymbol) {
		return EventRelevance{}, fmt.Errorf("%w %q", ErrUnsupportedInstrument, canonicalSymbol)
	}
	if !supportedRelevanceRegion(event.Region) {
		return EventRelevance{}, fmt.Errorf("%w %q", ErrUnsupportedRegion, event.Region)
	}
	if !supportedRelevanceEventType(event.Type) {
		return EventRelevance{}, fmt.Errorf("%w %q", ErrUnsupportedEventType, event.Type)
	}

	return EventRelevance{
		Symbol:   canonicalSymbol,
		Event:    event,
		Relevant: canonicalSymbol == "EURUSD" || event.Region == calendar.RegionUnitedStates,
	}, nil
}

func supportedRelevanceInstrument(symbol string) bool {
	return symbol == "SPY" || symbol == "DXY" || symbol == "EURUSD"
}

func supportedRelevanceRegion(region calendar.Region) bool {
	return region == calendar.RegionUnitedStates || region == calendar.RegionEurozone
}

func supportedRelevanceEventType(eventType calendar.EventType) bool {
	switch eventType {
	case calendar.EventTypeInflation,
		calendar.EventTypeEmployment,
		calendar.EventTypeInterestRateDecision,
		calendar.EventTypeGDP,
		calendar.EventTypePMI,
		calendar.EventTypeRetailSales:
		return true
	default:
		return false
	}
}
