package watchlist

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

func TestLinkRelevantEventsClassifiesAndPersistsInDeterministicOrder(t *testing.T) {
	eurozoneEvent := distinctStoredEvent(
		storedEventFixture(calendar.RegionEurozone, calendar.EventTypeInterestRateDecision),
		"00000000-0000-0000-0000-000000000058",
		"release-58",
		time.Hour,
	)
	unitedStatesEvent := distinctStoredEvent(
		storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeEmployment),
		"00000000-0000-0000-0000-000000000059",
		"release-59",
		2*time.Hour,
	)
	watchlistID := "00000000-0000-0000-0000-000000000005"
	reader := &watchlistReaderStub{stored: StoredWatchlist{
		ID: watchlistID,
		Definition: Definition{
			Name:    "Macro events",
			Symbols: []string{"DXY", "EURUSD", "SPY"},
		},
	}}
	canonicalLinks := []StoredEventLink{
		storedEventLinkFixture("DXY", unitedStatesEvent),
		storedEventLinkFixture("EURUSD", eurozoneEvent),
		storedEventLinkFixture("EURUSD", unitedStatesEvent),
		storedEventLinkFixture("SPY", unitedStatesEvent),
	}
	writer := &eventLinkWriterStub{links: canonicalLinks}
	events := []calendar.StoredEvent{eurozoneEvent, unitedStatesEvent}

	got, err := LinkRelevantEvents(t.Context(), reader, writer, watchlistID, events, "classifier")
	if err != nil {
		t.Fatalf("LinkRelevantEvents() error = %v", err)
	}
	if !reflect.DeepEqual(got, canonicalLinks) {
		t.Errorf("LinkRelevantEvents() = %#v, want canonical links %#v", got, canonicalLinks)
	}
	if reader.calls != 1 || reader.id != watchlistID {
		t.Errorf("watchlist lookup = (%d, %q), want (1, %q)", reader.calls, reader.id, watchlistID)
	}
	wantClassifications := []EventRelevance{
		{Symbol: "DXY", Event: unitedStatesEvent, Relevant: true},
		{Symbol: "EURUSD", Event: eurozoneEvent, Relevant: true},
		{Symbol: "EURUSD", Event: unitedStatesEvent, Relevant: true},
		{Symbol: "SPY", Event: unitedStatesEvent, Relevant: true},
	}
	if writer.calls != 1 || writer.watchlistID != watchlistID || writer.actor != "classifier" ||
		!reflect.DeepEqual(writer.classifications, wantClassifications) {
		t.Errorf(
			"event link write = (%d, %q, %#v, %q), want (1, %q, %#v, classifier)",
			writer.calls,
			writer.watchlistID,
			writer.classifications,
			writer.actor,
			watchlistID,
			wantClassifications,
		)
	}
}

func TestLinkRelevantEventsDelegatesEmptyAndRepeatedWrites(t *testing.T) {
	event := storedEventFixture(calendar.RegionEurozone, calendar.EventTypeInflation)
	reader := &watchlistReaderStub{stored: StoredWatchlist{
		ID:         "00000000-0000-0000-0000-000000000006",
		Definition: Definition{Symbols: []string{"DXY"}},
	}}
	want := []StoredEventLink{}
	writer := &eventLinkWriterStub{links: want}

	for range 2 {
		got, err := LinkRelevantEvents(
			t.Context(), reader, writer, reader.stored.ID, []calendar.StoredEvent{event}, "retry-actor",
		)
		if err != nil {
			t.Fatalf("LinkRelevantEvents() error = %v", err)
		}
		if got == nil || !reflect.DeepEqual(got, want) {
			t.Errorf("LinkRelevantEvents() = %#v, want non-nil empty result", got)
		}
	}
	if reader.calls != 2 || writer.calls != 2 {
		t.Errorf("dependency calls = (%d, %d), want (2, 2)", reader.calls, writer.calls)
	}
	if writer.classifications == nil || len(writer.classifications) != 0 {
		t.Errorf("persisted classifications = %#v, want non-nil empty result", writer.classifications)
	}
}

func TestLinkRelevantEventsStopsAfterLookupFailure(t *testing.T) {
	wantErr := errors.New("watchlist database unavailable")
	reader := &watchlistReaderStub{err: wantErr}

	got, err := LinkRelevantEvents(
		t.Context(), reader, panicEventLinkWriter{}, "watchlist-id", nil, "actor",
	)
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "retrieve watchlist for event linking") {
		t.Fatalf("LinkRelevantEvents() error = %v, want contextual lookup failure", err)
	}
	if got != nil {
		t.Errorf("LinkRelevantEvents() = %#v, want nil result", got)
	}
}

func TestLinkRelevantEventsStopsAfterClassificationFailure(t *testing.T) {
	validEvent := storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeInflation)
	tests := []struct {
		name    string
		symbols []string
		events  []calendar.StoredEvent
		wantErr error
	}{
		{name: "instrument", symbols: []string{"QQQ"}, events: []calendar.StoredEvent{validEvent}, wantErr: ErrUnsupportedInstrument},
		{name: "region", symbols: []string{"SPY"}, events: []calendar.StoredEvent{withEvent(validEvent, func(event *calendar.StoredEvent) { event.Region = "asia" })}, wantErr: ErrUnsupportedRegion},
		{name: "event type", symbols: []string{"SPY"}, events: []calendar.StoredEvent{withEvent(validEvent, func(event *calendar.StoredEvent) { event.Type = "earnings" })}, wantErr: ErrUnsupportedEventType},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &watchlistReaderStub{stored: StoredWatchlist{
				ID:         "00000000-0000-0000-0000-000000000007",
				Definition: Definition{Symbols: test.symbols},
			}}

			got, err := LinkRelevantEvents(
				t.Context(), reader, panicEventLinkWriter{}, reader.stored.ID, test.events, "actor",
			)
			if !errors.Is(err, test.wantErr) || !strings.Contains(err.Error(), "classify watchlist events") {
				t.Fatalf("LinkRelevantEvents() error = %v, want contextual %v", err, test.wantErr)
			}
			if got != nil {
				t.Errorf("LinkRelevantEvents() = %#v, want nil result", got)
			}
		})
	}
}

func TestLinkRelevantEventsPreservesPersistenceFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "repository", err: errors.New("event link database unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &watchlistReaderStub{stored: StoredWatchlist{
				ID:         "00000000-0000-0000-0000-000000000008",
				Definition: Definition{Symbols: []string{"SPY"}},
			}}
			writer := &eventLinkWriterStub{err: test.err}

			got, err := LinkRelevantEvents(
				t.Context(), reader, writer, reader.stored.ID,
				[]calendar.StoredEvent{storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeGDP)},
				"actor",
			)
			if !errors.Is(err, test.err) || !strings.Contains(err.Error(), "persist classified watchlist event links") {
				t.Fatalf("LinkRelevantEvents() error = %v, want contextual %v", err, test.err)
			}
			if got != nil {
				t.Errorf("LinkRelevantEvents() = %#v, want nil result", got)
			}
		})
	}
}

type watchlistReaderStub struct {
	stored StoredWatchlist
	err    error
	calls  int
	id     string
}

func (reader *watchlistReaderStub) Watchlist(_ context.Context, id string) (StoredWatchlist, error) {
	reader.calls++
	reader.id = id
	return reader.stored, reader.err
}

type eventLinkWriterStub struct {
	links           []StoredEventLink
	err             error
	calls           int
	watchlistID     string
	classifications []EventRelevance
	actor           string
}

func (writer *eventLinkWriterStub) CreateEventLinks(
	_ context.Context,
	watchlistID string,
	classifications []EventRelevance,
	actor string,
) ([]StoredEventLink, error) {
	writer.calls++
	writer.watchlistID = watchlistID
	writer.classifications = classifications
	writer.actor = actor
	return writer.links, writer.err
}

type panicEventLinkWriter struct{}

func (panicEventLinkWriter) CreateEventLinks(
	context.Context,
	string,
	[]EventRelevance,
	string,
) ([]StoredEventLink, error) {
	panic("event link persistence must not run")
}

func storedEventLinkFixture(symbol string, event calendar.StoredEvent) StoredEventLink {
	return StoredEventLink{
		ID:          "link-" + symbol + "-" + event.ID,
		WatchlistID: "00000000-0000-0000-0000-000000000005",
		Symbol:      symbol,
		Event:       event,
		CreatedAt:   event.CreatedAt.Add(time.Hour),
		UpdatedAt:   event.UpdatedAt.Add(time.Hour),
		CreatedBy:   "classifier",
		UpdatedBy:   "classifier",
	}
}
