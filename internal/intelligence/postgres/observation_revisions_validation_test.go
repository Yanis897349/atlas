package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

func TestRepositoryValidatesObservationRevisionsQueryBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	tests := []struct {
		name                string
		eventID             string
		source              string
		sourceObservationID string
		limit               int
		contains            string
	}{
		{name: "missing event ID", source: "source", sourceObservationID: "identity", limit: 1, contains: "event ID"},
		{name: "invalid event ID", eventID: "not-a-uuid", source: "source", sourceObservationID: "identity", limit: 1, contains: "event ID"},
		{name: "missing source", eventID: validEventID, source: " ", sourceObservationID: "identity", limit: 1, contains: "source is required"},
		{name: "missing source observation ID", eventID: validEventID, source: "source", sourceObservationID: "\t", limit: 1, contains: "source observation ID"},
		{name: "zero limit", eventID: validEventID, source: "source", sourceObservationID: "identity", contains: "limit must be between"},
		{name: "negative limit", eventID: validEventID, source: "source", sourceObservationID: "identity", limit: -1, contains: "limit must be between"},
		{name: "limit above maximum", eventID: validEventID, source: "source", sourceObservationID: "identity", limit: intelligence.MaxEventObservationsLimit + 1, contains: "limit must be between"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			revisions, err := repository.ObservationRevisions(
				t.Context(),
				test.eventID,
				test.source,
				test.sourceObservationID,
				test.limit,
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ObservationRevisions() error = %v, want %q", err, test.contains)
			}
			if revisions != nil {
				t.Errorf("ObservationRevisions() = %#v, want nil", revisions)
			}
		})
	}
}

func TestRepositoryPreservesObservationRevisionDatabaseFailures(t *testing.T) {
	wantErr := errors.New("database unavailable")
	repository, err := NewRepository(failureDB{err: wantErr})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if _, err := repository.ObservationRevisions(t.Context(), validEventID, "source", "identity", 1); err == nil || !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "begin economic event observation revision retrieval") {
		t.Fatalf("ObservationRevisions() error = %v, want contextual database failure", err)
	}

	canceledRepository, _ := NewRepository(failureDB{err: context.Canceled})
	if _, err := canceledRepository.ObservationRevisions(t.Context(), validEventID, "source", "identity", 1); !errors.Is(err, context.Canceled) {
		t.Errorf("ObservationRevisions() error = %v, want context.Canceled", err)
	}
}
