package postgres

import (
	"reflect"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

func TestScanEventNormalizesTimestampsToUTC(t *testing.T) {
	location := time.FixedZone("CI Local", -7*60*60)
	event := calendar.StoredEvent{
		ID: "event-1",
		Event: calendar.Event{
			Source:          "source",
			ExternalEventID: "external-1",
			Name:            "Economic event",
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeGDP,
			ScheduledAt:     time.Date(2026, time.August, 1, 1, 0, 0, 0, location),
			SourceURL:       "https://example.com/event",
			RetrievedAt:     time.Date(2026, time.July, 31, 1, 0, 0, 0, location),
		},
		CreatedAt: time.Date(2026, time.July, 30, 1, 0, 0, 0, location),
		UpdatedAt: time.Date(2026, time.July, 31, 2, 0, 0, 0, location),
		CreatedBy: "creator",
		UpdatedBy: "updater",
	}

	got, err := scanEvent(storedEventRow{event: event})
	if err != nil {
		t.Fatalf("scanEvent() error = %v", err)
	}
	for name, value := range map[string]time.Time{
		"scheduled": got.ScheduledAt,
		"retrieved": got.RetrievedAt,
		"created":   got.CreatedAt,
		"updated":   got.UpdatedAt,
	} {
		if value.Location() != time.UTC {
			t.Errorf("scanEvent() %s location = %v, want UTC", name, value.Location())
		}
	}
	if !got.ScheduledAt.Equal(event.ScheduledAt) || !got.RetrievedAt.Equal(event.RetrievedAt) ||
		!got.CreatedAt.Equal(event.CreatedAt) || !got.UpdatedAt.Equal(event.UpdatedAt) {
		t.Errorf("scanEvent() changed timestamp instants: %#v", got)
	}
}

type storedEventRow struct {
	event calendar.StoredEvent
}

func (row storedEventRow) Scan(destinations ...any) error {
	values := []any{
		row.event.ID,
		row.event.Source,
		row.event.ExternalEventID,
		row.event.Name,
		row.event.Region,
		row.event.Type,
		row.event.ScheduledAt,
		row.event.SourceURL,
		row.event.RetrievedAt,
		row.event.CreatedAt,
		row.event.UpdatedAt,
		row.event.CreatedBy,
		row.event.UpdatedBy,
	}
	for index, value := range values {
		reflect.ValueOf(destinations[index]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}
