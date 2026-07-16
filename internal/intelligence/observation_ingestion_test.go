package intelligence

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIngestObservationsPersistsAdapterSnapshotsInOrder(t *testing.T) {
	observedAt := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)
	observations := []Observation{
		{
			EconomicEventID:     "00000000-0000-0000-0000-000000000001",
			Source:              "official-statistics",
			SourceObservationID: "cpi-2026-06",
			SourceURL:           "https://example.com/releases/cpi-2026-06",
			ObservedAt:          observedAt,
			Consensus:           observationValue(" 3.2% "),
			Previous:            observationValue("3.1%"),
		},
		{
			EconomicEventID:     "00000000-0000-0000-0000-000000000002",
			Source:              "official-statistics",
			SourceObservationID: "employment-2026-06",
			SourceURL:           "https://example.com/releases/employment-2026-06",
			ObservedAt:          observedAt.Add(time.Minute),
			Actual:              observationValue("147,000"),
		},
	}
	adapter := &observationAdapterStub{observations: observations}
	persistence := &observationPersistenceStub{}

	count, err := IngestObservations(t.Context(), adapter, persistence, 17, " observation-ingestion ")
	if err != nil {
		t.Fatalf("IngestObservations() error = %v", err)
	}
	if count != len(observations) {
		t.Errorf("IngestObservations() count = %d, want %d", count, len(observations))
	}
	if adapter.calls != 1 || adapter.limit != 17 {
		t.Errorf("FetchObservations() call = (%d, %d), want (1, 17)", adapter.calls, adapter.limit)
	}
	if !reflect.DeepEqual(persistence.observations, observations) {
		t.Errorf("persisted observations = %#v, want exact adapter order %#v", persistence.observations, observations)
	}
	if !reflect.DeepEqual(persistence.actors, []string{"observation-ingestion", "observation-ingestion"}) {
		t.Errorf("persistence actors = %#v, want normalized actor for every observation", persistence.actors)
	}
}

func TestIngestObservationsRejectsInvalidInputBeforeDependencies(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		actor    string
		contains string
	}{
		{name: "zero limit", limit: 0, actor: "actor", contains: "limit must be between"},
		{name: "negative limit", limit: -1, actor: "actor", contains: "limit must be between"},
		{name: "limit above maximum", limit: MaxObservationIngestionLimit + 1, actor: "actor", contains: "limit must be between"},
		{name: "blank actor", limit: 1, actor: " \t", contains: "actor is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			count, err := IngestObservations(
				t.Context(),
				panicObservationAdapter{},
				panicObservationPersistence{},
				test.limit,
				test.actor,
			)
			if count != 0 || err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("IngestObservations() = (%d, %v), want zero count and %q", count, err, test.contains)
			}
		})
	}
}

func TestIngestObservationsHandlesEmptyAdapterResult(t *testing.T) {
	adapter := &observationAdapterStub{}

	count, err := IngestObservations(
		t.Context(),
		adapter,
		panicObservationPersistence{},
		MaxObservationIngestionLimit,
		"actor",
	)
	if err != nil {
		t.Fatalf("IngestObservations() error = %v", err)
	}
	if count != 0 {
		t.Errorf("IngestObservations() count = %d, want 0", count)
	}
	if adapter.calls != 1 || adapter.limit != MaxObservationIngestionLimit {
		t.Errorf(
			"FetchObservations() call = (%d, %d), want (1, %d)",
			adapter.calls,
			adapter.limit,
			MaxObservationIngestionLimit,
		)
	}
}

func TestIngestObservationsRejectsAdapterResultsAboveLimitBeforePersistence(t *testing.T) {
	adapter := &observationAdapterStub{observations: []Observation{
		{SourceObservationID: "first"},
		{SourceObservationID: "second"},
	}}

	count, err := IngestObservations(
		t.Context(),
		adapter,
		panicObservationPersistence{},
		1,
		"actor",
	)
	if count != 0 || err == nil ||
		!strings.Contains(err.Error(), "adapter returned 2 observations for limit 1") {
		t.Fatalf("IngestObservations() = (%d, %v), want zero count and oversized-result error", count, err)
	}
	if adapter.calls != 1 || adapter.limit != 1 {
		t.Errorf("FetchObservations() call = (%d, %d), want (1, 1)", adapter.calls, adapter.limit)
	}
}

func TestIngestObservationsPreservesFetchFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "adapter failure", err: errors.New("source unavailable")},
		{name: "cancellation", err: context.Canceled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			count, err := IngestObservations(
				t.Context(),
				&observationAdapterStub{err: test.err},
				panicObservationPersistence{},
				1,
				"actor",
			)
			if count != 0 || !errors.Is(err, test.err) ||
				!strings.Contains(err.Error(), "fetch economic event observations") {
				t.Fatalf("IngestObservations() = (%d, %v), want contextual %v", count, err, test.err)
			}
		})
	}
}

func TestIngestObservationsStopsAfterPersistenceFailure(t *testing.T) {
	observations := []Observation{
		{SourceObservationID: "first"},
		{SourceObservationID: "second"},
		{SourceObservationID: "third"},
	}
	persistErr := errors.New("database unavailable")
	persistence := &observationPersistenceStub{failAt: 2, err: persistErr}

	count, err := IngestObservations(
		t.Context(),
		&observationAdapterStub{observations: observations},
		persistence,
		3,
		"actor",
	)
	if count != 1 || !errors.Is(err, persistErr) ||
		!strings.Contains(err.Error(), "persist economic event observation 2") {
		t.Fatalf("IngestObservations() = (%d, %v), want one processed observation and contextual failure", count, err)
	}
	if !reflect.DeepEqual(persistence.observations, observations[:2]) {
		t.Errorf("attempted observations = %#v, want %#v", persistence.observations, observations[:2])
	}
}

func TestIngestObservationsPreservesPersistenceCancellation(t *testing.T) {
	observation := Observation{SourceObservationID: "first"}

	count, err := IngestObservations(
		t.Context(),
		&observationAdapterStub{observations: []Observation{observation}},
		&observationPersistenceStub{failAt: 1, err: context.Canceled},
		1,
		"actor",
	)
	if count != 0 || !errors.Is(err, context.Canceled) ||
		!strings.Contains(err.Error(), "persist economic event observation 1") {
		t.Fatalf("IngestObservations() = (%d, %v), want contextual cancellation", count, err)
	}
}

type observationAdapterStub struct {
	observations []Observation
	err          error
	calls        int
	limit        int
}

func (adapter *observationAdapterStub) FetchObservations(_ context.Context, limit int) ([]Observation, error) {
	adapter.calls++
	adapter.limit = limit
	return adapter.observations, adapter.err
}

type observationPersistenceStub struct {
	observations []Observation
	actors       []string
	failAt       int
	err          error
}

func (persistence *observationPersistenceStub) UpsertObservation(
	_ context.Context,
	observation Observation,
	actor string,
) (StoredObservation, error) {
	persistence.observations = append(persistence.observations, observation)
	persistence.actors = append(persistence.actors, actor)
	if persistence.err != nil && len(persistence.observations) == persistence.failAt {
		return StoredObservation{}, persistence.err
	}
	return StoredObservation{Observation: observation}, nil
}

type panicObservationAdapter struct{}

func (panicObservationAdapter) FetchObservations(context.Context, int) ([]Observation, error) {
	panic("unexpected observation adapter call")
}

type panicObservationPersistence struct{}

func (panicObservationPersistence) UpsertObservation(
	context.Context,
	Observation,
	string,
) (StoredObservation, error) {
	panic("unexpected observation persistence call")
}

func observationValue(value string) *string {
	return &value
}
