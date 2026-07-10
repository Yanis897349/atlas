package ingestion_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestIngestPersistsFetchedRecords(t *testing.T) {
	records := []ingestion.SourceRecord{{SourceItemID: "first"}, {SourceItemID: "second"}}
	repository := &recordingRepository{}

	count, err := ingestion.Ingest(t.Context(), staticFetcher{records: records}, repository, "rss-ingestion")
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if count != len(records) {
		t.Errorf("Ingest() count = %d, want %d", count, len(records))
	}
	if len(repository.records) != len(records) || repository.actor != "rss-ingestion" {
		t.Errorf("persisted records = %#v with actor %q", repository.records, repository.actor)
	}
}

func TestIngestReportsFetchAndPersistenceFailures(t *testing.T) {
	t.Run("fetch", func(t *testing.T) {
		_, err := ingestion.Ingest(t.Context(), staticFetcher{err: context.Canceled}, &recordingRepository{}, "actor")
		if err == nil || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "fetch source records") {
			t.Fatalf("Ingest() error = %v, want contextual cancellation error", err)
		}
	})

	t.Run("persist", func(t *testing.T) {
		repository := &recordingRepository{err: errors.New("database unavailable")}
		count, err := ingestion.Ingest(t.Context(), staticFetcher{records: []ingestion.SourceRecord{{}}}, repository, "actor")
		if count != 0 || err == nil || !strings.Contains(err.Error(), "persist source record 1: database unavailable") {
			t.Fatalf("Ingest() = (%d, %v), want first-record persistence error", count, err)
		}
	})
}

type staticFetcher struct {
	records []ingestion.SourceRecord
	err     error
}

func (fetcher staticFetcher) Fetch(context.Context) ([]ingestion.SourceRecord, error) {
	return fetcher.records, fetcher.err
}

type recordingRepository struct {
	records []ingestion.SourceRecord
	actor   string
	err     error
}

func (repository *recordingRepository) PersistSourceRecord(
	_ context.Context,
	record ingestion.SourceRecord,
	actor string,
) error {
	repository.records = append(repository.records, record)
	repository.actor = actor
	return repository.err
}
