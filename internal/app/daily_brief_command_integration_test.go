package app

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

func TestRunReadsDailyBriefInputEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: applicationDatabaseEnv(database.URL)}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	sourceRepository, err := ingestionpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(source records) error = %v", err)
	}
	eventRepository, err := calendarpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(events) error = %v", err)
	}

	publicationStart := time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC)
	publicationEnd := publicationStart.Add(4 * time.Hour)
	records := []ingestion.SourceRecord{
		commandSourceRecord("before", publicationStart.Add(-time.Microsecond)),
		commandSourceRecord("start", publicationStart),
		commandSourceRecord("middle-b", publicationStart.Add(2*time.Hour)),
		commandSourceRecord("middle-a", publicationStart.Add(2*time.Hour)),
		commandSourceRecord("end", publicationEnd),
		commandSourceRecord("after", publicationEnd.Add(time.Microsecond)),
	}
	storedRecords := make(map[string]ingestionpostgres.StoredSourceRecord, len(records))
	for _, record := range records {
		stored, persistErr := sourceRepository.UpsertSourceRecord(t.Context(), record, "rss-ingestion")
		if persistErr != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", record.SourceItemID, persistErr)
		}
		storedRecords[record.SourceItemID] = stored
	}

	eventStart := time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC)
	eventEnd := eventStart.Add(4 * time.Hour)
	events := []calendar.Event{
		commandEvent("before", calendar.RegionUnitedStates, eventStart.Add(-time.Microsecond)),
		commandEvent("start", calendar.RegionUnitedStates, eventStart),
		commandEvent("middle-b", calendar.RegionUnitedStates, eventStart.Add(2*time.Hour)),
		commandEvent("middle-a", calendar.RegionUnitedStates, eventStart.Add(2*time.Hour)),
		commandEvent("end", calendar.RegionUnitedStates, eventEnd),
		commandEvent("after", calendar.RegionUnitedStates, eventEnd.Add(time.Microsecond)),
		commandEvent("other-region", calendar.RegionEurozone, eventStart.Add(time.Hour)),
	}
	storedEvents := make(map[string]calendarpostgres.StoredEvent, len(events))
	for _, event := range events {
		stored, persistErr := eventRepository.UpsertEvent(t.Context(), event, "calendar-ingestion")
		if persistErr != nil {
			t.Fatalf("UpsertEvent(%q) error = %v", event.ExternalEventID, persistErr)
		}
		storedEvents[event.ExternalEventID] = stored
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	err = Run(t.Context(), []string{
		"daily-brief-input",
		"--region", "united_states",
		"--publication-from", "2026-07-11T10:00:00+02:00",
		"--publication-to", "2026-07-11T14:00:00+02:00",
		"--source-record-limit", "3",
		"--event-from", "2026-07-12T10:00:00+02:00",
		"--event-to", "2026-07-12T14:00:00+02:00",
		"--upcoming-event-limit", "2",
	}, dependencies)
	if err != nil {
		t.Fatalf("Run(daily-brief-input) error = %v", err)
	}

	var output dailyBriefInputOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command JSON: %v", err)
	}
	if output.Region != calendar.RegionUnitedStates {
		t.Errorf("output region = %q, want %q", output.Region, calendar.RegionUnitedStates)
	}
	if output.PublicationWindow.From != "2026-07-11T08:00:00Z" || output.PublicationWindow.To != "2026-07-11T12:00:00Z" {
		t.Errorf("publication window = %#v, want UTC inclusive window", output.PublicationWindow)
	}
	if output.EventWindow.From != "2026-07-12T08:00:00Z" || output.EventWindow.To != "2026-07-12T12:00:00Z" {
		t.Errorf("event window = %#v, want UTC inclusive window", output.EventWindow)
	}

	middleRecordIDs := []string{storedRecords["middle-a"].ID, storedRecords["middle-b"].ID}
	sort.Strings(middleRecordIDs)
	wantRecordIDs := []string{storedRecords["end"].ID, middleRecordIDs[0], middleRecordIDs[1]}
	if len(output.SourceRecords) != len(wantRecordIDs) {
		t.Fatalf("source record count = %d, want %d", len(output.SourceRecords), len(wantRecordIDs))
	}
	for index, wantID := range wantRecordIDs {
		if output.SourceRecords[index].ID != wantID || output.SourceRecords[index].OriginalURL == "" {
			t.Errorf("source_records[%d] = %#v, want ID %q with citation", index, output.SourceRecords[index], wantID)
		}
		if !strings.HasSuffix(output.SourceRecords[index].PublishedAt, "Z") || !strings.HasSuffix(output.SourceRecords[index].RetrievedAt, "Z") {
			t.Errorf("source_records[%d] times = (%q, %q), want UTC", index, output.SourceRecords[index].PublishedAt, output.SourceRecords[index].RetrievedAt)
		}
	}

	middleEventIDs := []string{storedEvents["middle-a"].ID, storedEvents["middle-b"].ID}
	sort.Strings(middleEventIDs)
	wantEventIDs := []string{storedEvents["start"].ID, middleEventIDs[0]}
	if len(output.UpcomingEvents) != len(wantEventIDs) {
		t.Fatalf("upcoming event count = %d, want %d", len(output.UpcomingEvents), len(wantEventIDs))
	}
	for index, wantID := range wantEventIDs {
		if output.UpcomingEvents[index].ID != wantID || output.UpcomingEvents[index].SourceURL == "" {
			t.Errorf("upcoming_events[%d] = %#v, want ID %q with citation", index, output.UpcomingEvents[index], wantID)
		}
	}

	stdout.Reset()
	err = Run(t.Context(), []string{
		"daily-brief-input",
		"--region", "united_states",
		"--publication-from", "2026-07-11T08:00:00Z",
		"--publication-to", "2026-07-11T12:00:00Z",
		"--source-record-limit", "10",
		"--event-from", "2026-07-12T08:00:00Z",
		"--event-to", "2026-07-12T12:00:00Z",
		"--upcoming-event-limit", "10",
	}, dependencies)
	if err != nil {
		t.Fatalf("Run(daily-brief-input inclusive windows) error = %v", err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode inclusive command JSON: %v", err)
	}
	if len(output.SourceRecords) != 4 || output.SourceRecords[0].SourceItemID != "end" || output.SourceRecords[len(output.SourceRecords)-1].SourceItemID != "start" {
		t.Errorf("inclusive source record boundaries = %#v, want end through start", output.SourceRecords)
	}
	if len(output.UpcomingEvents) != 4 || output.UpcomingEvents[0].ExternalEventID != "start" || output.UpcomingEvents[len(output.UpcomingEvents)-1].ExternalEventID != "end" {
		t.Errorf("inclusive event boundaries = %#v, want start through end", output.UpcomingEvents)
	}
}

func commandSourceRecord(sourceItemID string, publishedAt time.Time) ingestion.SourceRecord {
	return ingestion.SourceRecord{
		Source:       "example-news",
		SourceItemID: sourceItemID,
		OriginalURL:  "https://example.com/news/" + sourceItemID,
		Title:        "Source record " + sourceItemID,
		PublishedAt:  publishedAt,
		RetrievedAt:  time.Date(2026, time.July, 11, 14, 0, 0, 0, time.UTC),
	}
}
