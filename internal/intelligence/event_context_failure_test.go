package intelligence

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/search"
	"github.com/jackc/pgx/v5"
)

func TestAssembleEventContextRejectsInvalidInputBeforeDependencies(t *testing.T) {
	valid := validEventContextQuery()
	tests := []struct {
		name     string
		query    EventContextQuery
		contains string
	}{
		{name: "missing event ID", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.EventID = "" }), contains: "event ID must be a UUID"},
		{name: "malformed event ID", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.EventID = "not-a-uuid" }), contains: "event ID must be a UUID"},
		{name: "invalid event ID separators", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.EventID = "00000000X0000X0000X0000X000000000083" }), contains: "event ID must be a UUID"},
		{name: "missing publication start", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.PublicationWindowStart = time.Time{} }), contains: "publication window start is required"},
		{name: "missing publication end", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.PublicationWindowEnd = time.Time{} }), contains: "publication window end is required"},
		{name: "reversed publication window", query: withEventContextQuery(valid, func(query *EventContextQuery) {
			query.PublicationWindowEnd = query.PublicationWindowStart.Add(-time.Nanosecond)
		}), contains: "publication window end must not be before start"},
		{name: "zero source record limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.SourceRecordLimit = 0 }), contains: "source record limit must be between"},
		{name: "negative source record limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.SourceRecordLimit = -1 }), contains: "source record limit must be between"},
		{name: "source record limit above maximum", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.SourceRecordLimit = search.MaxSimilarSourceRecordsLimit + 1 }), contains: "source record limit must be between"},
		{name: "zero observation limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.ObservationLimit = 0 }), contains: "observation limit must be between"},
		{name: "negative observation limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.ObservationLimit = -1 }), contains: "observation limit must be between"},
		{name: "observation limit above maximum", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.ObservationLimit = MaxEventObservationsLimit + 1 }), contains: "observation limit must be between"},
		{name: "zero observation revision limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.ObservationRevisionLimit = 0 }), contains: "observation revision limit must be between"},
		{name: "negative observation revision limit", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.ObservationRevisionLimit = -1 }), contains: "observation revision limit must be between"},
		{name: "observation revision limit above maximum", query: withEventContextQuery(valid, func(query *EventContextQuery) { query.ObservationRevisionLimit = MaxEventObservationsLimit + 1 }), contains: "observation revision limit must be between"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AssembleEventContext(t.Context(), panicEconomicEventReader{}, panicObservationReader{}, panicObservationRevisionReader{}, panicEmbedder{}, panicSimilarSourceRecordReader{}, test.query)
			if err == nil || !strings.Contains(err.Error(), "validate economic event context query") || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("AssembleEventContext() error = %v, want validation containing %q", err, test.contains)
			}
			if !reflect.DeepEqual(got, EventContext{}) {
				t.Errorf("AssembleEventContext() = %#v, want zero context", got)
			}
		})
	}
}

func TestAssembleEventContextPreservesDependencyFailures(t *testing.T) {
	tests := []struct {
		name                                                        string
		eventErr, observationErr, revisionErr, embedErr, sourcesErr error
		contains                                                    string
	}{
		{name: "event repository", eventErr: errors.New("calendar unavailable"), contains: "retrieve economic event"},
		{name: "event not found", eventErr: pgx.ErrNoRows, contains: "retrieve economic event"},
		{name: "event cancellation", eventErr: context.Canceled, contains: "retrieve economic event"},
		{name: "observation repository", observationErr: errors.New("observations unavailable"), contains: "retrieve economic event observations"},
		{name: "observation cancellation", observationErr: context.Canceled, contains: "retrieve economic event observations"},
		{name: "observation revision repository", revisionErr: errors.New("revisions unavailable"), contains: "retrieve economic event observation revisions"},
		{name: "observation revision cancellation", revisionErr: context.Canceled, contains: "retrieve economic event observation revisions"},
		{name: "embedding provider", embedErr: errors.New("provider unavailable"), contains: "embed semantic search query with provider"},
		{name: "embedding cancellation", embedErr: context.Canceled, contains: "embed semantic search query with provider"},
		{name: "source repository", sourcesErr: errors.New("search unavailable"), contains: "retrieve similar source records"},
		{name: "source cancellation", sourcesErr: context.Canceled, contains: "retrieve similar source records"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := &economicEventReaderStub{event: storedEventFixture(validEventID, "Inflation"), err: test.eventErr}
			observationResults := []StoredObservation{}
			if test.revisionErr != nil {
				observationResults = []StoredObservation{storedObservationFixture("00000000-0000-0000-0000-000000000004", validEventID, "revision-failure", time.Now())}
			}
			observations := &observationReaderStub{results: observationResults, err: test.observationErr}
			revisions := &observationRevisionReaderStub{err: test.revisionErr}
			embedder := &embedderStub{batch: validEmbeddingBatch(), err: test.embedErr}
			sources := &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}, err: test.sourcesErr}
			got, err := AssembleEventContext(t.Context(), events, observations, revisions, embedder, sources, validEventContextQuery())
			wantErr := firstEventContextFailure(test.eventErr, test.observationErr, test.revisionErr, test.embedErr, test.sourcesErr)
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), test.contains) || !strings.Contains(err.Error(), failureContext(test.eventErr, test.observationErr, test.revisionErr)) {
				t.Fatalf("AssembleEventContext() error = %v, want contextual %v", err, wantErr)
			}
			if !reflect.DeepEqual(got, EventContext{}) {
				t.Errorf("AssembleEventContext() = %#v, want zero context", got)
			}
			if test.eventErr != nil && (observations.calls != 0 || embedder.calls != 0 || sources.calls != 0) {
				t.Errorf("downstream calls after event failure = (%d, %d, %d), want none", observations.calls, embedder.calls, sources.calls)
			}
			if (test.observationErr != nil || test.revisionErr != nil) && (embedder.calls != 0 || sources.calls != 0) {
				t.Errorf("downstream semantic calls after observation failure = (%d, %d), want none", embedder.calls, sources.calls)
			}
			if test.embedErr != nil && sources.calls != 0 {
				t.Errorf("source calls after embedding failure = %d, want none", sources.calls)
			}
		})
	}
}

func TestAssembleEventContextPreservesMalformedEmbeddingFailure(t *testing.T) {
	got, err := AssembleEventContext(t.Context(), &economicEventReaderStub{event: storedEventFixture(validEventID, "Inflation")}, &observationReaderStub{results: []StoredObservation{}}, &observationRevisionReaderStub{}, &embedderStub{batch: search.EmbeddingBatch{Provider: "provider", Model: "model"}}, panicSimilarSourceRecordReader{}, validEventContextQuery())
	if err == nil || !strings.Contains(err.Error(), "retrieve economic event source records") || !strings.Contains(err.Error(), "validate semantic search query embedding") {
		t.Fatalf("AssembleEventContext() error = %v, want contextual malformed embedding failure", err)
	}
	if !reflect.DeepEqual(got, EventContext{}) {
		t.Errorf("AssembleEventContext() = %#v, want zero context", got)
	}
}

func firstEventContextFailure(failures ...error) error {
	for _, failure := range failures {
		if failure != nil {
			return failure
		}
	}
	return nil
}
