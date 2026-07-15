package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryValidatesObservationBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	valid := validObservation()
	tests := []struct {
		name        string
		observation intelligence.Observation
		actor       string
		contains    string
	}{
		{name: "invalid event ID", observation: withObservation(valid, func(value *intelligence.Observation) { value.EconomicEventID = "not-a-uuid" }), actor: "worker", contains: "event ID"},
		{name: "missing source", observation: withObservation(valid, func(value *intelligence.Observation) { value.Source = " " }), actor: "worker", contains: "source is required"},
		{name: "missing source observation ID", observation: withObservation(valid, func(value *intelligence.Observation) { value.SourceObservationID = "" }), actor: "worker", contains: "source observation ID"},
		{name: "invalid source URL", observation: withObservation(valid, func(value *intelligence.Observation) { value.SourceURL = "/release" }), actor: "worker", contains: "source URL"},
		{name: "missing observation time", observation: withObservation(valid, func(value *intelligence.Observation) { value.ObservedAt = time.Time{} }), actor: "worker", contains: "observation time"},
		{name: "missing values", observation: withObservation(valid, func(value *intelligence.Observation) { value.Consensus = nil }), actor: "worker", contains: "at least one"},
		{name: "blank consensus", observation: withObservation(valid, func(value *intelligence.Observation) { value.Consensus = stringPointer(" \t") }), actor: "worker", contains: "consensus value"},
		{name: "blank previous", observation: withObservation(valid, func(value *intelligence.Observation) { value.Consensus = nil; value.Previous = stringPointer(" ") }), actor: "worker", contains: "previous value"},
		{name: "blank actual", observation: withObservation(valid, func(value *intelligence.Observation) { value.Consensus = nil; value.Actual = stringPointer("") }), actor: "worker", contains: "actual value"},
		{name: "missing actor", observation: valid, actor: " ", contains: "actor"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := repository.UpsertObservation(t.Context(), test.observation, test.actor)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("UpsertObservation() error = %v, want %q", err, test.contains)
			}
			if got != (intelligence.StoredObservation{}) {
				t.Errorf("UpsertObservation() = %#v, want zero observation", got)
			}
		})
	}
}

func TestRepositoryValidatesEventObservationsQueryBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	tests := []struct {
		name    string
		eventID string
		limit   int
	}{
		{name: "missing event ID", limit: 1},
		{name: "invalid event ID", eventID: "not-a-uuid", limit: 1},
		{name: "zero limit", eventID: validEventID},
		{name: "negative limit", eventID: validEventID, limit: -1},
		{name: "limit above maximum", eventID: validEventID, limit: intelligence.MaxEventObservationsLimit + 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observations, err := repository.EventObservations(t.Context(), test.eventID, test.limit)
			if err == nil {
				t.Fatal("EventObservations() error = nil, want validation error")
			}
			if observations != nil {
				t.Errorf("EventObservations() = %#v, want nil", observations)
			}
		})
	}
}

func TestRepositoryPreservesObservationDatabaseFailures(t *testing.T) {
	wantErr := errors.New("database unavailable")
	repository, err := NewRepository(failureDB{err: wantErr})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if _, err := repository.UpsertObservation(t.Context(), validObservation(), "worker"); err == nil || !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "upsert economic event observation") {
		t.Fatalf("UpsertObservation() error = %v, want contextual database failure", err)
	}
	if _, err := repository.EventObservations(t.Context(), validEventID, 1); err == nil || !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "begin economic event observation retrieval") {
		t.Fatalf("EventObservations() error = %v, want contextual database failure", err)
	}

	canceledRepository, _ := NewRepository(failureDB{err: context.Canceled})
	if _, err := canceledRepository.UpsertObservation(t.Context(), validObservation(), "worker"); !errors.Is(err, context.Canceled) {
		t.Errorf("UpsertObservation() error = %v, want context.Canceled", err)
	}
	if _, err := canceledRepository.EventObservations(t.Context(), validEventID, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("EventObservations() error = %v, want context.Canceled", err)
	}
}

func TestNewRepositoryRequiresDatabase(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository() error = nil, want missing database error")
	}
}

const validEventID = "00000000-0000-0000-0000-000000000086"

func validObservation() intelligence.Observation {
	return intelligence.Observation{
		EconomicEventID:     validEventID,
		Source:              "official-statistics",
		SourceObservationID: "cpi-2026-06",
		SourceURL:           "https://example.com/releases/cpi-2026-06",
		ObservedAt:          time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC),
		Consensus:           stringPointer("3.2%"),
	}
}

func withObservation(
	observation intelligence.Observation,
	update func(*intelligence.Observation),
) intelligence.Observation {
	update(&observation)
	return observation
}

func stringPointer(value string) *string {
	return &value
}

type panicDB struct{}

func (panicDB) Begin(context.Context) (pgx.Tx, error) {
	panic("validation must happen before beginning a PostgreSQL transaction")
}

func (panicDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("validation must happen before querying PostgreSQL")
}

func (panicDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("validation must happen before querying PostgreSQL")
}

type failureDB struct {
	err error
}

func (database failureDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, database.err
}

func (database failureDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, database.err
}

func (database failureDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return failureRow(database)
}

type failureRow struct {
	err error
}

func (row failureRow) Scan(...any) error {
	return row.err
}
