package intelligencecmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/search"
)

func TestRunEconomicEventContextPreservesFailuresWithoutBufferedOutput(t *testing.T) {
	wantErr := errors.New("dependency unavailable")
	tests := []struct {
		name         string
		events       intelligence.EconomicEventReader
		observations intelligence.ObservationReader
		revisions    intelligence.ObservationRevisionReader
		embedder     search.Embedder
		sources      search.SimilarSourceRecordReader
		stdout       io.Writer
		contains     string
		wantErr      error
	}{
		{name: "event repository", events: &economicEventReaderStub{err: wantErr}, observations: panicObservationReader{}, revisions: panicObservationRevisionReader{}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event", wantErr: wantErr},
		{name: "cancellation", events: &economicEventReaderStub{err: context.Canceled}, observations: panicObservationReader{}, revisions: panicObservationRevisionReader{}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event", wantErr: context.Canceled},
		{name: "observation repository", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: &observationReaderStub{err: wantErr}, revisions: panicObservationRevisionReader{}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event observations", wantErr: wantErr},
		{name: "observation revision repository", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: &observationReaderStub{results: []intelligence.StoredObservation{{Observation: intelligence.Observation{EconomicEventID: validEventID, Source: "source", SourceObservationID: "identity"}}}}, revisions: &observationRevisionReaderStub{err: wantErr}, embedder: panicEmbedder{}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "retrieve economic event observation revisions", wantErr: wantErr},
		{name: "provider", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{err: wantErr}, sources: panicSimilarSourceRecordReader{}, stdout: &bytes.Buffer{}, contains: "embed semantic search query", wantErr: wantErr},
		{name: "source repository", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{err: wantErr}, stdout: &bytes.Buffer{}, contains: "retrieve similar source records", wantErr: wantErr},
		{name: "encoding", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{{CosineDistance: math.NaN()}}}, stdout: &bytes.Buffer{}, contains: "encode economic event context"},
		{name: "writer", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}}, stdout: errorWriter{err: wantErr}, contains: "write economic event context", wantErr: wantErr},
		{name: "short writer", events: &economicEventReaderStub{event: storedEventFixture("Inflation", time.Now())}, observations: emptyObservationReader(), revisions: &observationRevisionReaderStub{}, embedder: &embedderStub{batch: validEmbeddingBatch()}, sources: &similarSourceRecordReaderStub{results: []search.SimilarSourceRecord{}}, stdout: shortWriter{}, contains: "short write", wantErr: io.ErrShortWrite},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runEconomicEventContext(
				t.Context(),
				test.events,
				test.observations,
				test.revisions,
				test.embedder,
				test.sources,
				test.stdout,
				validEventContextQuery(),
			)
			if err == nil || !strings.Contains(err.Error(), test.contains) ||
				(test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Fatalf("error = %v, want contextual failure containing %q", err, test.contains)
			}
			if buffer, ok := test.stdout.(*bytes.Buffer); ok && buffer.Len() != 0 {
				t.Errorf("stdout = %q, want no JSON", buffer.String())
			}
		})
	}
}
