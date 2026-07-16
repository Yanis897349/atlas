package intelligencecmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarbls "github.com/Yanis897349/atlas/internal/calendar/bls"
	"github.com/Yanis897349/atlas/internal/intelligence"
)

func TestRunIngestBLSObservationsWritesCompleteCount(t *testing.T) {
	observations := []intelligence.Observation{
		blsObservationFixture(validCPIEventID, "CUUR0000SA0:2026-M06"),
		blsObservationFixture(validEmploymentEventID, "CES0000000001:2026-M06"),
	}
	adapter := &observationAdapterStub{observations: observations}
	persistence := &observationPersistenceStub{}
	stdout := &bytes.Buffer{}
	command := validBLSObservationIngestionCommand()

	if err := runIngestBLSObservations(
		t.Context(),
		validBLSObservationEventReader(),
		adapter,
		persistence,
		stdout,
		command,
	); err != nil {
		t.Fatalf("runIngestBLSObservations() error = %v", err)
	}
	if adapter.limit != 2 || !reflect.DeepEqual(persistence.observations, observations) {
		t.Errorf("ingestion = adapter limit %d, observations %#v; want complete adapter order", adapter.limit, persistence.observations)
	}
	if !reflect.DeepEqual(persistence.actors, []string{
		blsObservationIngestionActor,
		blsObservationIngestionActor,
	}) {
		t.Errorf("actors = %#v, want fixed BLS ingestion actor", persistence.actors)
	}
	if stdout.String() != "ingested 2 BLS economic event observations\n" {
		t.Errorf("stdout = %q, want complete count", stdout.String())
	}
}

func TestRunIngestBLSObservationsWritesEmptyCount(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runIngestBLSObservations(
		t.Context(),
		validBLSObservationEventReader(),
		&observationAdapterStub{observations: []intelligence.Observation{}},
		&observationPersistenceStub{},
		stdout,
		validBLSObservationIngestionCommand(),
	)
	if err != nil {
		t.Fatalf("runIngestBLSObservations() error = %v", err)
	}
	if stdout.String() != "ingested 0 BLS economic event observations\n" {
		t.Errorf("stdout = %q, want zero count", stdout.String())
	}
}

func TestRunIngestBLSObservationsRejectsInvalidEventBindingsBeforeDependencies(t *testing.T) {
	wantErr := errors.New("event repository unavailable")
	tests := []struct {
		name     string
		reader   *blsObservationEventReaderStub
		contains string
		wantErr  error
	}{
		{
			name: "event repository",
			reader: &blsObservationEventReaderStub{
				errors: map[string]error{validCPIEventID: wantErr},
			},
			contains: "retrieve CPI economic event",
			wantErr:  wantErr,
		},
		{
			name: "swapped releases",
			reader: &blsObservationEventReaderStub{events: map[string]calendar.StoredEvent{
				validCPIEventID: storedBLSEvent(
					validCPIEventID,
					blsEmploymentEventName,
					calendar.EventTypeEmployment,
				),
			}},
			contains: "must be the canonical BLS Consumer Price Index release",
		},
		{
			name: "non-BLS source",
			reader: &blsObservationEventReaderStub{events: map[string]calendar.StoredEvent{
				validCPIEventID: func() calendar.StoredEvent {
					event := storedBLSEvent(validCPIEventID, blsCPIEventName, calendar.EventTypeInflation)
					event.Source = "other-calendar"
					return event
				}(),
			}},
			contains: "must be the canonical BLS Consumer Price Index release",
		},
		{
			name: "wrong region",
			reader: &blsObservationEventReaderStub{events: map[string]calendar.StoredEvent{
				validCPIEventID: func() calendar.StoredEvent {
					event := storedBLSEvent(validCPIEventID, blsCPIEventName, calendar.EventTypeInflation)
					event.Region = calendar.RegionEurozone
					return event
				}(),
			}},
			contains: "must be the canonical BLS Consumer Price Index release",
		},
		{
			name: "invalid employment release",
			reader: &blsObservationEventReaderStub{events: map[string]calendar.StoredEvent{
				validCPIEventID: storedBLSEvent(validCPIEventID, blsCPIEventName, calendar.EventTypeInflation),
				validEmploymentEventID: storedBLSEvent(
					validEmploymentEventID,
					"Other Employment Report",
					calendar.EventTypeEmployment,
				),
			}},
			contains: "must be the canonical BLS Employment Situation release",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := &observationAdapterStub{}
			persistence := &observationPersistenceStub{}
			stdout := &bytes.Buffer{}
			err := runIngestBLSObservations(
				t.Context(),
				test.reader,
				adapter,
				persistence,
				stdout,
				validBLSObservationIngestionCommand(),
			)
			if err == nil || !strings.Contains(err.Error(), "after 0 processed observations") ||
				!strings.Contains(err.Error(), test.contains) ||
				(test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Fatalf("error = %v, want zero-count binding failure containing %q", err, test.contains)
			}
			if adapter.calls != 0 || len(persistence.observations) != 0 || stdout.Len() != 0 {
				t.Errorf(
					"dependencies/output = adapter %d, persistence %d, stdout %q; want untouched",
					adapter.calls,
					len(persistence.observations),
					stdout.String(),
				)
			}
		})
	}
}

func TestRunIngestBLSObservationsReportsPartialCountWithoutSuccessOutput(t *testing.T) {
	wantErr := errors.New("database unavailable")
	observations := []intelligence.Observation{
		blsObservationFixture(validCPIEventID, "cpi"),
		blsObservationFixture(validEmploymentEventID, "employment"),
	}
	stdout := &bytes.Buffer{}
	err := runIngestBLSObservations(
		t.Context(),
		validBLSObservationEventReader(),
		&observationAdapterStub{observations: observations},
		&observationPersistenceStub{failAt: 2, err: wantErr},
		stdout,
		validBLSObservationIngestionCommand(),
	)
	if err == nil || !errors.Is(err, wantErr) ||
		!strings.Contains(err.Error(), "after 1 processed observations") ||
		!strings.Contains(err.Error(), "persist economic event observation 2") {
		t.Fatalf("error = %v, want partial count and contextual persistence failure", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no premature success output", stdout.String())
	}
}

func TestRunIngestBLSObservationsPreservesCancellationWithoutSuccessOutput(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runIngestBLSObservations(
		t.Context(),
		validBLSObservationEventReader(),
		&observationAdapterStub{err: context.Canceled},
		&observationPersistenceStub{},
		stdout,
		validBLSObservationIngestionCommand(),
	)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "after 0 processed observations") {
		t.Fatalf("error = %v, want contextual cancellation with zero count", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no success output", stdout.String())
	}
}

func TestRunIngestBLSObservationsReportsOutputFailures(t *testing.T) {
	wantErr := errors.New("writer unavailable")
	tests := []struct {
		name    string
		stdout  io.Writer
		wantErr error
	}{
		{name: "writer", stdout: errorWriter{err: wantErr}, wantErr: wantErr},
		{name: "short writer", stdout: shortWriter{}, wantErr: io.ErrShortWrite},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runIngestBLSObservations(
				t.Context(),
				validBLSObservationEventReader(),
				&observationAdapterStub{observations: []intelligence.Observation{}},
				&observationPersistenceStub{},
				test.stdout,
				validBLSObservationIngestionCommand(),
			)
			if err == nil || !errors.Is(err, test.wantErr) ||
				!strings.Contains(err.Error(), "write BLS economic event observation ingestion result") {
				t.Fatalf("error = %v, want contextual %v", err, test.wantErr)
			}
		})
	}
}

func blsObservationFixture(eventID, sourceObservationID string) intelligence.Observation {
	actual := "100"
	return intelligence.Observation{
		EconomicEventID:     eventID,
		Source:              "bls",
		SourceObservationID: sourceObservationID,
		SourceURL:           "https://data.bls.gov/timeseries/series",
		ObservedAt:          time.Date(2026, time.July, 16, 20, 0, 0, 0, time.UTC),
		Actual:              &actual,
	}
}

func validBLSObservationIngestionCommand() ingestBLSObservationsCommand {
	return ingestBLSObservationsCommand{
		cpiEventID:        validCPIEventID,
		employmentEventID: validEmploymentEventID,
		limit:             2,
	}
}

type observationAdapterStub struct {
	observations []intelligence.Observation
	err          error
	limit        int
	calls        int
}

func (adapter *observationAdapterStub) FetchObservations(
	_ context.Context,
	limit int,
) ([]intelligence.Observation, error) {
	adapter.calls++
	adapter.limit = limit
	return adapter.observations, adapter.err
}

func validBLSObservationEventReader() *blsObservationEventReaderStub {
	return &blsObservationEventReaderStub{events: map[string]calendar.StoredEvent{
		validCPIEventID: storedBLSEvent(validCPIEventID, blsCPIEventName, calendar.EventTypeInflation),
		validEmploymentEventID: storedBLSEvent(
			validEmploymentEventID,
			blsEmploymentEventName,
			calendar.EventTypeEmployment,
		),
	}}
}

func storedBLSEvent(id, name string, eventType calendar.EventType) calendar.StoredEvent {
	return calendar.StoredEvent{
		ID: id,
		Event: calendar.Event{
			Source: calendarbls.Source,
			Name:   name,
			Region: calendar.RegionUnitedStates,
			Type:   eventType,
		},
	}
}

type blsObservationEventReaderStub struct {
	events map[string]calendar.StoredEvent
	errors map[string]error
	calls  []string
}

func (reader *blsObservationEventReaderStub) EconomicEvent(
	_ context.Context,
	id string,
) (calendar.StoredEvent, error) {
	reader.calls = append(reader.calls, id)
	return reader.events[id], reader.errors[id]
}

type observationPersistenceStub struct {
	observations []intelligence.Observation
	actors       []string
	failAt       int
	err          error
}

func (persistence *observationPersistenceStub) UpsertObservation(
	_ context.Context,
	observation intelligence.Observation,
	actor string,
) (intelligence.StoredObservation, error) {
	call := len(persistence.observations) + 1
	if persistence.failAt == call {
		return intelligence.StoredObservation{}, persistence.err
	}
	persistence.observations = append(persistence.observations, observation)
	persistence.actors = append(persistence.actors, actor)
	return intelligence.StoredObservation{Observation: observation}, nil
}
