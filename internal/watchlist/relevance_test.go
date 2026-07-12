package watchlist

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

func TestClassifyEventRelevanceSupportedMatrix(t *testing.T) {
	eventTypes := []calendar.EventType{
		calendar.EventTypeInflation,
		calendar.EventTypeEmployment,
		calendar.EventTypeInterestRateDecision,
		calendar.EventTypeGDP,
		calendar.EventTypePMI,
		calendar.EventTypeRetailSales,
	}
	regions := []calendar.Region{calendar.RegionUnitedStates, calendar.RegionEurozone}
	instruments := []struct {
		input     string
		canonical string
	}{
		{input: " spy ", canonical: "SPY"},
		{input: "dxy", canonical: "DXY"},
		{input: "EURUSD", canonical: "EURUSD"},
	}

	for _, instrument := range instruments {
		for _, region := range regions {
			for _, eventType := range eventTypes {
				event := storedEventFixture(region, eventType)
				got, err := ClassifyEventRelevance(instrument.input, event)
				if err != nil {
					t.Fatalf("ClassifyEventRelevance(%q, %q, %q) error = %v", instrument.input, region, eventType, err)
				}

				wantRelevant := instrument.canonical == "EURUSD" || region == calendar.RegionUnitedStates
				want := EventRelevance{Symbol: instrument.canonical, Event: event, Relevant: wantRelevant}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("ClassifyEventRelevance(%q, %q, %q) = %#v, want %#v", instrument.input, region, eventType, got, want)
				}
			}
		}
	}
}

func TestClassifyEventRelevancePreservesCanonicalEvent(t *testing.T) {
	event := storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeInterestRateDecision)

	got, err := ClassifyEventRelevance("SPY", event)
	if err != nil {
		t.Fatalf("ClassifyEventRelevance() error = %v", err)
	}
	if !reflect.DeepEqual(got.Event, event) {
		t.Errorf("classified event = %#v, want unchanged canonical event %#v", got.Event, event)
	}
}

func TestClassifyEventRelevanceRejectsUnsupportedInputs(t *testing.T) {
	validEvent := storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeInflation)
	tests := []struct {
		name    string
		symbol  string
		event   calendar.StoredEvent
		wantErr error
	}{
		{name: "blank instrument", symbol: " \t\n", event: validEvent, wantErr: ErrUnsupportedInstrument},
		{name: "unknown instrument", symbol: "QQQ", event: validEvent, wantErr: ErrUnsupportedInstrument},
		{name: "unsupported region", symbol: "SPY", event: withEvent(validEvent, func(event *calendar.StoredEvent) { event.Region = "asia" }), wantErr: ErrUnsupportedRegion},
		{name: "unsupported event type", symbol: "SPY", event: withEvent(validEvent, func(event *calendar.StoredEvent) { event.Type = "earnings" }), wantErr: ErrUnsupportedEventType},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ClassifyEventRelevance(test.symbol, test.event)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ClassifyEventRelevance() error = %v, want error matching %v", err, test.wantErr)
			}
			if !reflect.DeepEqual(got, EventRelevance{}) {
				t.Errorf("ClassifyEventRelevance() result = %#v, want zero result", got)
			}
		})
	}
}

func storedEventFixture(region calendar.Region, eventType calendar.EventType) calendar.StoredEvent {
	scheduledAt := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.UTC)
	return calendar.StoredEvent{
		ID: "00000000-0000-0000-0000-000000000054",
		Event: calendar.Event{
			Source:          "official-calendar",
			ExternalEventID: "release-54",
			Name:            "Supported economic release",
			Region:          region,
			Type:            eventType,
			ScheduledAt:     scheduledAt,
			SourceURL:       "https://example.com/calendar/release-54",
			RetrievedAt:     scheduledAt.Add(-24 * time.Hour),
		},
		CreatedAt: scheduledAt.Add(-23 * time.Hour),
		UpdatedAt: scheduledAt.Add(-22 * time.Hour),
		CreatedBy: "calendar-worker",
		UpdatedBy: "calendar-worker",
	}
}

func withEvent(event calendar.StoredEvent, update func(*calendar.StoredEvent)) calendar.StoredEvent {
	update(&event)
	return event
}
