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

func TestLinkRelevantEventCandidatesRetrievesClassifiesAndPersistsInDeterministicOrder(t *testing.T) {
	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	windowEnd := windowStart.Add(6 * time.Hour)
	eurozoneEvent := distinctStoredEvent(
		storedEventFixture(calendar.RegionEurozone, calendar.EventTypeInterestRateDecision),
		"00000000-0000-0000-0000-000000000060",
		"release-60",
		time.Hour,
	)
	unitedStatesEvent := distinctStoredEvent(
		storedEventFixture(calendar.RegionUnitedStates, calendar.EventTypeEmployment),
		"00000000-0000-0000-0000-000000000061",
		"release-61",
		2*time.Hour,
	)
	events := []calendar.StoredEvent{eurozoneEvent, unitedStatesEvent}
	candidates := &eventCandidateReaderStub{events: events}
	watchlistID := "00000000-0000-0000-0000-000000000009"
	reader := &watchlistReaderStub{stored: StoredWatchlist{
		ID: watchlistID,
		Definition: Definition{
			Name:    "Macro events",
			Symbols: []string{"DXY", "EURUSD", "SPY"},
		},
	}}
	wantLinks := []StoredEventLink{
		storedEventLinkFixture("DXY", unitedStatesEvent),
		storedEventLinkFixture("EURUSD", eurozoneEvent),
		storedEventLinkFixture("EURUSD", unitedStatesEvent),
		storedEventLinkFixture("SPY", unitedStatesEvent),
	}
	writer := &eventLinkWriterStub{links: wantLinks}

	got, err := LinkRelevantEventCandidates(
		t.Context(), candidates, reader, writer, watchlistID, windowStart, windowEnd, 24, "classifier",
	)
	if err != nil {
		t.Fatalf("LinkRelevantEventCandidates() error = %v", err)
	}
	if !reflect.DeepEqual(got, wantLinks) {
		t.Errorf("LinkRelevantEventCandidates() = %#v, want %#v", got, wantLinks)
	}
	if candidates.calls != 1 || candidates.windowStart != windowStart || candidates.windowEnd != windowEnd || candidates.limit != 24 {
		t.Errorf(
			"candidate retrieval = (%d, %v, %v, %d), want (1, %v, %v, 24)",
			candidates.calls,
			candidates.windowStart,
			candidates.windowEnd,
			candidates.limit,
			windowStart,
			windowEnd,
		)
	}
	wantClassifications := []EventRelevance{
		{Symbol: "DXY", Event: unitedStatesEvent, Relevant: true},
		{Symbol: "EURUSD", Event: eurozoneEvent, Relevant: true},
		{Symbol: "EURUSD", Event: unitedStatesEvent, Relevant: true},
		{Symbol: "SPY", Event: unitedStatesEvent, Relevant: true},
	}
	if !reflect.DeepEqual(writer.classifications, wantClassifications) {
		t.Errorf("persisted classifications = %#v, want %#v", writer.classifications, wantClassifications)
	}
}

func TestLinkRelevantEventCandidatesDelegatesEmptyCandidateResults(t *testing.T) {
	candidates := &eventCandidateReaderStub{events: []calendar.StoredEvent{}}
	reader := &watchlistReaderStub{stored: StoredWatchlist{
		ID:         "00000000-0000-0000-0000-000000000010",
		Definition: Definition{Symbols: []string{"SPY"}},
	}}
	writer := &eventLinkWriterStub{links: []StoredEventLink{}}
	windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)

	got, err := LinkRelevantEventCandidates(
		t.Context(), candidates, reader, writer, reader.stored.ID,
		windowStart, windowStart, 1, "classifier",
	)
	if err != nil {
		t.Fatalf("LinkRelevantEventCandidates() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("LinkRelevantEventCandidates() = %#v, want non-nil empty result", got)
	}
	if reader.calls != 1 || writer.calls != 1 || writer.classifications == nil || len(writer.classifications) != 0 {
		t.Errorf(
			"downstream calls and classifications = (%d, %d, %#v), want (1, 1, non-nil empty)",
			reader.calls,
			writer.calls,
			writer.classifications,
		)
	}
}

func TestLinkRelevantEventCandidatesStopsBeforeLinkingAfterRetrievalFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "repository", err: errors.New("calendar database unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
			got, err := LinkRelevantEventCandidates(
				t.Context(), &eventCandidateReaderStub{err: test.err}, panicWatchlistReader{},
				panicEventLinkWriter{}, "watchlist-id", windowStart, windowStart.Add(time.Hour), 10, "actor",
			)
			if !errors.Is(err, test.err) || !strings.Contains(err.Error(), "retrieve watchlist event candidates") {
				t.Fatalf("LinkRelevantEventCandidates() error = %v, want contextual %v", err, test.err)
			}
			if got != nil {
				t.Errorf("LinkRelevantEventCandidates() = %#v, want nil result", got)
			}
		})
	}
}

func TestLinkRelevantEventCandidatesPreservesContextualLinkingFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "repository", err: errors.New("watchlist database unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			windowStart := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
			got, err := LinkRelevantEventCandidates(
				t.Context(), &eventCandidateReaderStub{events: []calendar.StoredEvent{}},
				&watchlistReaderStub{err: test.err}, panicEventLinkWriter{}, "watchlist-id",
				windowStart, windowStart.Add(time.Hour), 10, "actor",
			)
			if !errors.Is(err, test.err) ||
				!strings.Contains(err.Error(), "link retrieved watchlist event candidates") ||
				!strings.Contains(err.Error(), "retrieve watchlist for event linking") {
				t.Fatalf("LinkRelevantEventCandidates() error = %v, want contextual %v", err, test.err)
			}
			if got != nil {
				t.Errorf("LinkRelevantEventCandidates() = %#v, want nil result", got)
			}
		})
	}
}

type eventCandidateReaderStub struct {
	events      []calendar.StoredEvent
	err         error
	calls       int
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (reader *eventCandidateReaderStub) WatchlistEventCandidates(
	_ context.Context,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]calendar.StoredEvent, error) {
	reader.calls++
	reader.windowStart = windowStart
	reader.windowEnd = windowEnd
	reader.limit = limit
	return reader.events, reader.err
}

type panicWatchlistReader struct{}

func (panicWatchlistReader) Watchlist(context.Context, string) (StoredWatchlist, error) {
	panic("watchlist retrieval must not run")
}
