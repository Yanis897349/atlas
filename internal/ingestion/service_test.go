package ingestion_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestIngestPersistsFetchedRecords(t *testing.T) {
	records := []ingestion.SourceRecord{{SourceItemID: "first"}, {SourceItemID: "second"}}
	repository := &recordingRepository{stored: []ingestion.StoredSourceRecord{
		{ID: "stored-first", SourceRecord: records[0]},
		{ID: "stored-second", SourceRecord: records[1]},
	}}

	stored, err := ingestion.Ingest(t.Context(), staticFetcher{records: records}, repository, "rss-ingestion")
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if !reflect.DeepEqual(stored, repository.stored) {
		t.Errorf("Ingest() = %#v, want canonical records %#v", stored, repository.stored)
	}
	if len(repository.records) != len(records) || repository.actor != "rss-ingestion" {
		t.Errorf("persisted records = %#v with actor %q", repository.records, repository.actor)
	}
}

func TestIngestReturnsNonNilEmptyRecords(t *testing.T) {
	stored, err := ingestion.Ingest(t.Context(), staticFetcher{}, &recordingRepository{}, "actor")
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if stored == nil || len(stored) != 0 {
		t.Errorf("Ingest() = %#v, want non-nil empty records", stored)
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
		records := []ingestion.SourceRecord{{SourceItemID: "first"}, {SourceItemID: "second"}}
		first := ingestion.StoredSourceRecord{ID: "stored-first", SourceRecord: records[0]}
		repository := &recordingRepository{
			stored:    []ingestion.StoredSourceRecord{first},
			err:       errors.New("database unavailable"),
			failAfter: 1,
		}
		stored, err := ingestion.Ingest(t.Context(), staticFetcher{records: records}, repository, "actor")
		if !reflect.DeepEqual(stored, []ingestion.StoredSourceRecord{first}) || err == nil ||
			!strings.Contains(err.Error(), "persist source record 2: database unavailable") {
			t.Fatalf("Ingest() = (%#v, %v), want stored prefix and second-record persistence error", stored, err)
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
	records   []ingestion.SourceRecord
	stored    []ingestion.StoredSourceRecord
	actor     string
	err       error
	failAfter int
}

func (repository *recordingRepository) UpsertSourceRecord(
	_ context.Context,
	record ingestion.SourceRecord,
	actor string,
) (ingestion.StoredSourceRecord, error) {
	repository.records = append(repository.records, record)
	repository.actor = actor
	index := len(repository.records) - 1
	if repository.err != nil && index >= repository.failAfter {
		return ingestion.StoredSourceRecord{}, repository.err
	}
	return repository.stored[index], nil
}
