package intelligencecmd

import (
	"context"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

type observationRevisionReaderStub struct {
	results             []intelligence.StoredObservation
	resultsByCall       [][]intelligence.StoredObservation
	err                 error
	eventID             string
	source              string
	sourceObservationID string
	limit               int
	calls               []observationRevisionReaderInput
}

type observationRevisionReaderInput struct {
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
	reader.calls = append(reader.calls, observationRevisionReaderInput{
		eventID: eventID, source: source, sourceObservationID: sourceObservationID, limit: limit,
	})
	if reader.err != nil {
		return nil, reader.err
	}
	if reader.resultsByCall != nil {
		return reader.resultsByCall[len(reader.calls)-1], nil
	}
	return reader.results, nil
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}
