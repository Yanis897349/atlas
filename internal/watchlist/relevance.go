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

// ClassifyWatchlistEvents returns relevant classifications in symbol-then-event input order.
func ClassifyWatchlistEvents(symbols []string, events []calendar.StoredEvent) ([]EventRelevance, error) {
	if len(events) == 0 {
		for symbolIndex, symbol := range symbols {
			if _, err := validateRelevanceInstrument(symbol); err != nil {
				return nil, fmt.Errorf("classify watchlist symbol %d: %w", symbolIndex, err)
			}
		}
	}
	if len(symbols) == 0 {
		for eventIndex, event := range events {
			if err := validateRelevanceEvent(event); err != nil {
				return nil, fmt.Errorf("classify watchlist event %d: %w", eventIndex, err)
			}
		}
	}

	classifications := make([]EventRelevance, 0)
	for symbolIndex, symbol := range symbols {
		for eventIndex, event := range events {
			classification, err := ClassifyEventRelevance(symbol, event)
			if err != nil {
				return nil, fmt.Errorf(
					"classify watchlist symbol %d event %d: %w",
					symbolIndex,
					eventIndex,
					err,
				)
			}
			if classification.Relevant {
				classifications = append(classifications, classification)
			}
		}
	}

	return classifications, nil
}

// ClassifyEventRelevance applies the supported deterministic instrument-to-region rules.
func ClassifyEventRelevance(symbol string, event calendar.StoredEvent) (EventRelevance, error) {
	canonicalSymbol, err := validateRelevanceInstrument(symbol)
	if err != nil {
		return EventRelevance{}, err
	}
	if err := validateRelevanceEvent(event); err != nil {
		return EventRelevance{}, err
	}

	return EventRelevance{
		Symbol:   canonicalSymbol,
		Event:    event,
		Relevant: canonicalSymbol == "EURUSD" || event.Region == calendar.RegionUnitedStates,
	}, nil
}

func validateRelevanceInstrument(symbol string) (string, error) {
	canonicalSymbol := NormalizeInstrumentSymbol(symbol)
	if !supportedRelevanceInstrument(canonicalSymbol) {
		return "", fmt.Errorf("%w %q", ErrUnsupportedInstrument, canonicalSymbol)
	}
	return canonicalSymbol, nil
}

func validateRelevanceEvent(event calendar.StoredEvent) error {
	if !supportedRelevanceRegion(event.Region) {
		return fmt.Errorf("%w %q", ErrUnsupportedRegion, event.Region)
	}
	if !supportedRelevanceEventType(event.Type) {
		return fmt.Errorf("%w %q", ErrUnsupportedEventType, event.Type)
	}
	return nil
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
