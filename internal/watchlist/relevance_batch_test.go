package watchlist

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

func TestClassifyWatchlistEventsFiltersInSymbolThenEventOrder(t *testing.T) {
	eurozoneEvent := distinctStoredEvent(
		storedEventFixture(calendar.RegionEurozone, calendar.EventTypeInterestRateDecision),
		"00000000-0000-0000-0000-000000000055",
		"release-55",
		time.Hour,
	)
	unitedStatesEvent := distinctStoredEvent(
		storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeEmployment),
		"00000000-0000-0000-0000-000000000056",
		"release-56",
		2*time.Hour,
	)

	got, err := ClassifyWatchlistEvents(
		[]string{" dxy ", "eurusd", "SPY"},
		[]calendar.StoredEvent{eurozoneEvent, unitedStatesEvent},
	)
	if err != nil {
		t.Fatalf("ClassifyWatchlistEvents() error = %v", err)
	}

	want := []EventRelevance{
		{Symbol: "DXY", Event: unitedStatesEvent, Relevant: true},
		{Symbol: "EURUSD", Event: eurozoneEvent, Relevant: true},
		{Symbol: "EURUSD", Event: unitedStatesEvent, Relevant: true},
		{Symbol: "SPY", Event: unitedStatesEvent, Relevant: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClassifyWatchlistEvents() = %#v, want %#v", got, want)
	}
}

func TestClassifyWatchlistEventsReturnsNonNilEmptyResult(t *testing.T) {
	event := storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeInflation)
	tests := []struct {
		name    string
		symbols []string
		events  []calendar.StoredEvent
	}{
		{name: "nil inputs"},
		{name: "no symbols", symbols: []string{}, events: []calendar.StoredEvent{event}},
		{name: "no events", symbols: []string{"SPY"}, events: []calendar.StoredEvent{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ClassifyWatchlistEvents(test.symbols, test.events)
			if err != nil {
				t.Fatalf("ClassifyWatchlistEvents() error = %v", err)
			}
			if got == nil || len(got) != 0 {
				t.Errorf("ClassifyWatchlistEvents() = %#v, want non-nil empty result", got)
			}
		})
	}
}

func TestClassifyWatchlistEventsReturnsContextualUnsupportedInputFailures(t *testing.T) {
	validEvent := storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeInflation)
	tests := []struct {
		name        string
		symbols     []string
		events      []calendar.StoredEvent
		wantErr     error
		wantContext string
	}{
		{
			name:        "instrument after a relevant classification",
			symbols:     []string{"SPY", "QQQ"},
			events:      []calendar.StoredEvent{validEvent},
			wantErr:     ErrUnsupportedInstrument,
			wantContext: "symbol 1 event 0",
		},
		{
			name:        "instrument without events",
			symbols:     []string{"QQQ"},
			events:      []calendar.StoredEvent{},
			wantErr:     ErrUnsupportedInstrument,
			wantContext: "symbol 0",
		},
		{
			name:    "event region",
			symbols: []string{"SPY"},
			events: []calendar.StoredEvent{
				validEvent,
				withEvent(validEvent, func(event *calendar.StoredEvent) { event.Region = "asia" }),
			},
			wantErr:     ErrUnsupportedRegion,
			wantContext: "symbol 0 event 1",
		},
		{
			name:    "event type",
			symbols: []string{"SPY"},
			events: []calendar.StoredEvent{
				validEvent,
				withEvent(validEvent, func(event *calendar.StoredEvent) { event.Type = "earnings" }),
			},
			wantErr:     ErrUnsupportedEventType,
			wantContext: "symbol 0 event 1",
		},
		{
			name:    "event without instruments",
			symbols: []string{},
			events: []calendar.StoredEvent{
				withEvent(validEvent, func(event *calendar.StoredEvent) { event.Region = "asia" }),
			},
			wantErr:     ErrUnsupportedRegion,
			wantContext: "event 0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ClassifyWatchlistEvents(test.symbols, test.events)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ClassifyWatchlistEvents() error = %v, want error matching %v", err, test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantContext) {
				t.Errorf("ClassifyWatchlistEvents() error = %q, want context %q", err, test.wantContext)
			}
			if got != nil {
				t.Errorf("ClassifyWatchlistEvents() result = %#v, want nil result", got)
			}
		})
	}
}

func distinctStoredEvent(event calendar.StoredEvent, id string, externalID string, offset time.Duration) calendar.StoredEvent {
	event.ID = id
	event.ExternalEventID = externalID
	event.ScheduledAt = event.ScheduledAt.Add(offset)
	return event
}
