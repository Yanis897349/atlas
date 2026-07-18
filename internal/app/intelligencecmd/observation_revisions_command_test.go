package intelligencecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

func TestRunObservationRevisionsWritesCompleteOrderedJSON(t *testing.T) {
	paris := time.FixedZone("Paris", 2*60*60)
	base := time.Date(2026, time.July, 18, 12, 0, 0, 123456789, paris)
	consensus, previous, actual := "3.1%", "3.0%", "3.3%"
	stored := []intelligence.StoredObservation{
		{
			ID: "00000000-0000-0000-0000-000000000002",
			Observation: intelligence.Observation{
				EconomicEventID:     validEventID,
				Source:              "official-statistics",
				SourceObservationID: "cpi-2026-07",
				SourceURL:           "https://example.com/releases/cpi-revised",
				ObservedAt:          base.Add(time.Hour),
				Previous:            &previous,
				Actual:              &actual,
			},
			CreatedAt: base.Add(2 * time.Hour),
			UpdatedAt: base.Add(3 * time.Hour),
			CreatedBy: "revision-worker",
			UpdatedBy: "revision-worker",
		},
		{
			ID: "00000000-0000-0000-0000-000000000001",
			Observation: intelligence.Observation{
				EconomicEventID:     validEventID,
				Source:              "official-statistics",
				SourceObservationID: "cpi-2026-07",
				SourceURL:           "https://example.com/releases/cpi",
				ObservedAt:          base,
				Consensus:           &consensus,
			},
			CreatedAt: base.Add(time.Minute),
			UpdatedAt: base.Add(2 * time.Minute),
			CreatedBy: "initial-worker",
			UpdatedBy: "initial-worker",
		},
	}
	reader := &observationRevisionReaderStub{results: stored}
	stdout := &bytes.Buffer{}
	query := observationRevisionsQuery{
		eventID:             validEventID,
		source:              "official-statistics",
		sourceObservationID: "cpi-2026-07",
		limit:               2,
	}

	if err := runObservationRevisions(t.Context(), reader, stdout, query); err != nil {
		t.Fatalf("runObservationRevisions() error = %v", err)
	}
	if reader.eventID != query.eventID || reader.source != query.source ||
		reader.sourceObservationID != query.sourceObservationID || reader.limit != query.limit {
		t.Errorf("reader query = %#v, want %#v", reader, query)
	}

	var got []economicEventObservationOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(got) != 2 || got[0].ID != stored[0].ID || got[1].ID != stored[1].ID ||
		got[0].EconomicEventID != validEventID || got[0].Source != "official-statistics" ||
		got[0].SourceObservationID != "cpi-2026-07" ||
		got[0].SourceURL != "https://example.com/releases/cpi-revised" ||
		got[0].ObservedAt != "2026-07-18T11:00:00.123456789Z" ||
		got[0].Consensus != nil || got[0].Previous == nil || *got[0].Previous != previous ||
		got[0].Actual == nil || *got[0].Actual != actual ||
		got[0].CreatedAt != "2026-07-18T12:00:00.123456789Z" ||
		got[0].UpdatedAt != "2026-07-18T13:00:00.123456789Z" ||
		got[0].CreatedBy != "revision-worker" || got[0].UpdatedBy != "revision-worker" ||
		got[1].Consensus == nil || *got[1].Consensus != consensus ||
		got[1].Previous != nil || got[1].Actual != nil {
		t.Errorf("output = %#v, want complete UTC revisions in repository order", got)
	}
	if !strings.Contains(stdout.String(), `"consensus":null`) ||
		!strings.Contains(stdout.String(), `"previous":null`) ||
		!strings.Contains(stdout.String(), `"actual":null`) {
		t.Errorf("stdout = %q, want exact nullable values", stdout.String())
	}
}

func TestRunObservationRevisionsWritesEmptyArray(t *testing.T) {
	stdout := &bytes.Buffer{}
	err := runObservationRevisions(
		t.Context(),
		&observationRevisionReaderStub{},
		stdout,
		observationRevisionsQuery{eventID: validEventID, source: "source", sourceObservationID: "identity", limit: 10},
	)
	if err != nil {
		t.Fatalf("runObservationRevisions() error = %v", err)
	}
	var got []economicEventObservationOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got == nil || len(got) != 0 || stdout.String() != "[]\n" {
		t.Errorf("output = (%#v, %q), want non-nil empty JSON array", got, stdout.String())
	}
}

func TestRunObservationRevisionsPreservesFailuresWithoutSuccessOutput(t *testing.T) {
	wantErr := errors.New("revisions unavailable")
	tests := []struct {
		name     string
		reader   intelligence.ObservationRevisionReader
		stdout   io.Writer
		contains string
		wantErr  error
	}{
		{name: "repository", reader: &observationRevisionReaderStub{err: wantErr}, stdout: &bytes.Buffer{}, contains: "retrieve economic event observation revisions", wantErr: wantErr},
		{name: "cancellation", reader: &observationRevisionReaderStub{err: context.Canceled}, stdout: &bytes.Buffer{}, contains: "retrieve economic event observation revisions", wantErr: context.Canceled},
		{name: "writer", reader: &observationRevisionReaderStub{}, stdout: errorWriter{err: wantErr}, contains: "write economic event observation revisions", wantErr: wantErr},
		{name: "short writer", reader: &observationRevisionReaderStub{}, stdout: shortWriter{}, contains: "short write", wantErr: io.ErrShortWrite},
	}
	query := observationRevisionsQuery{eventID: validEventID, source: "source", sourceObservationID: "identity", limit: 10}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runObservationRevisions(t.Context(), test.reader, test.stdout, query)
			if err == nil || !strings.Contains(err.Error(), test.contains) || !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want contextual failure containing %q", err, test.contains)
			}
			if buffer, ok := test.stdout.(*bytes.Buffer); ok && buffer.Len() != 0 {
				t.Errorf("stdout = %q, want no JSON", buffer.String())
			}
		})
	}
}

type observationRevisionReaderStub struct {
	results             []intelligence.StoredObservation
	err                 error
	eventID             string
	source              string
	sourceObservationID string
	limit               int
}

func (reader *observationRevisionReaderStub) ObservationRevisions(
	_ context.Context,
	eventID string,
	source string,
	sourceObservationID string,
	limit int,
) ([]intelligence.StoredObservation, error) {
	reader.eventID = eventID
	reader.source = source
	reader.sourceObservationID = sourceObservationID
	reader.limit = limit
	return reader.results, reader.err
}
