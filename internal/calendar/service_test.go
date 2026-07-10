package calendar_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/calendar"
)

func TestIngestPersistsFetchedEvents(t *testing.T) {
	events := []calendar.Event{
		{ExternalEventID: "first"},
		{ExternalEventID: "second"},
	}
	adapter := &staticAdapter{events: events}
	repository := &recordingRepository{}

	count, err := calendar.Ingest(t.Context(), adapter, repository, "calendar-ingestion")
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if count != len(events) {
		t.Errorf("Ingest() count = %d, want %d", count, len(events))
	}
	if adapter.calls != 1 {
		t.Errorf("FetchEvents() calls = %d, want 1", adapter.calls)
	}
	if !reflect.DeepEqual(repository.events, events) {
		t.Errorf("persisted events = %#v, want %#v", repository.events, events)
	}
	if !reflect.DeepEqual(repository.actors, []string{"calendar-ingestion", "calendar-ingestion"}) {
		t.Errorf("persistence actors = %#v, want calendar-ingestion for every event", repository.actors)
	}
}

func TestIngestHandlesEmptyCalendar(t *testing.T) {
	adapter := &staticAdapter{}
	repository := &recordingRepository{}

	count, err := calendar.Ingest(t.Context(), adapter, repository, "calendar-ingestion")
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if count != 0 {
		t.Errorf("Ingest() count = %d, want 0", count)
	}
	if adapter.calls != 1 {
		t.Errorf("FetchEvents() calls = %d, want 1", adapter.calls)
	}
	if len(repository.events) != 0 {
		t.Errorf("persisted events = %#v, want none", repository.events)
	}
}

func TestIngestReportsFetchFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "adapter failure", err: errors.New("calendar unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &recordingRepository{}
			count, err := calendar.Ingest(t.Context(), &staticAdapter{err: test.err}, repository, "actor")
			if count != 0 || err == nil {
				t.Fatalf("Ingest() = (%d, %v), want zero count and fetch error", count, err)
			}
			if !errors.Is(err, test.err) || !strings.Contains(err.Error(), "fetch economic events") {
				t.Errorf("Ingest() error = %v, want wrapped %v with fetch context", err, test.err)
			}
			if len(repository.events) != 0 {
				t.Errorf("persisted events = %#v, want none", repository.events)
			}
		})
	}
}

func TestIngestStopsAfterPartialPersistenceFailure(t *testing.T) {
	persistErr := errors.New("database unavailable")
	events := []calendar.Event{
		{ExternalEventID: "first"},
		{ExternalEventID: "second"},
		{ExternalEventID: "third"},
	}
	repository := &recordingRepository{failAt: 2, err: persistErr}

	count, err := calendar.Ingest(t.Context(), &staticAdapter{events: events}, repository, "actor")
	if count != 1 || err == nil {
		t.Fatalf("Ingest() = (%d, %v), want one persisted event and error", count, err)
	}
	if !errors.Is(err, persistErr) || !strings.Contains(err.Error(), "persist economic event 2") {
		t.Errorf("Ingest() error = %v, want wrapped second-event persistence error", err)
	}
	if !reflect.DeepEqual(repository.attempted, events[:2]) {
		t.Errorf("attempted events = %#v, want %#v", repository.attempted, events[:2])
	}
	if !reflect.DeepEqual(repository.events, events[:1]) {
		t.Errorf("persisted events = %#v, want %#v", repository.events, events[:1])
	}
}

type staticAdapter struct {
	events []calendar.Event
	err    error
	calls  int
}

func (adapter *staticAdapter) FetchEvents(context.Context) ([]calendar.Event, error) {
	adapter.calls++
	return adapter.events, adapter.err
}

type recordingRepository struct {
	events    []calendar.Event
	attempted []calendar.Event
	actors    []string
	failAt    int
	err       error
}

func (repository *recordingRepository) PersistEvent(
	_ context.Context,
	event calendar.Event,
	actor string,
) error {
	repository.attempted = append(repository.attempted, event)
	if len(repository.attempted) == repository.failAt {
		return repository.err
	}
	repository.events = append(repository.events, event)
	repository.actors = append(repository.actors, actor)
	return nil
}
