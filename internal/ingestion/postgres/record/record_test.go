package record

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

func TestColumnsQualifiesCanonicalProjection(t *testing.T) {
	unqualified := Columns("")
	qualified := Columns(" source_records ")
	for _, column := range []string{"id::text", "source_item_id", "published_at", "updated_by"} {
		if !strings.Contains(unqualified, column) || strings.Contains(unqualified, "source_records."+column) {
			t.Errorf("Columns(\"\") = %q", unqualified)
		}
		if !strings.Contains(qualified, "source_records."+column) {
			t.Errorf("Columns(source_records) missing qualified %q: %q", column, qualified)
		}
	}
}

func TestScanPreservesValuesAndTimestampLocations(t *testing.T) {
	location := time.FixedZone("source", 3*60*60)
	want := ingestion.StoredSourceRecord{
		ID: "record-1",
		SourceRecord: ingestion.SourceRecord{
			Source: "publisher", SourceItemID: "item-1", OriginalURL: "https://example.com/1", Title: "Headline",
			PublishedAt: time.Date(2026, time.July, 12, 10, 0, 0, 0, location),
			RetrievedAt: time.Date(2026, time.July, 12, 11, 0, 0, 0, location),
		},
		CreatedAt: time.Date(2026, time.July, 12, 12, 0, 0, 0, location),
		UpdatedAt: time.Date(2026, time.July, 12, 13, 0, 0, 0, location),
		CreatedBy: "creator", UpdatedBy: "updater",
	}

	got, err := Scan(storedSourceRecordRow{stored: want})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() = %#v, want %#v", got, want)
	}

	NormalizeTimes(&got)
	for name, value := range map[string]time.Time{
		"published": got.PublishedAt, "retrieved": got.RetrievedAt,
		"created": got.CreatedAt, "updated": got.UpdatedAt,
	} {
		if value.Location() != time.UTC {
			t.Errorf("NormalizeTimes() %s location = %v, want UTC", name, value.Location())
		}
	}
	if !got.PublishedAt.Equal(want.PublishedAt) || !got.RetrievedAt.Equal(want.RetrievedAt) ||
		!got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("NormalizeTimes() changed timestamp instants: %#v", got)
	}
}

type storedSourceRecordRow struct {
	stored ingestion.StoredSourceRecord
}

func (row storedSourceRecordRow) Scan(destinations ...any) error {
	values := []any{
		row.stored.ID, row.stored.Source, row.stored.SourceItemID, row.stored.OriginalURL, row.stored.Title,
		row.stored.PublishedAt, row.stored.RetrievedAt, row.stored.CreatedAt, row.stored.UpdatedAt,
		row.stored.CreatedBy, row.stored.UpdatedBy,
	}
	for index, value := range values {
		reflect.ValueOf(destinations[index]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}
