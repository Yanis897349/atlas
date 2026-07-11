package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
)

type dailyBriefWindowOutput struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type dailyBriefSourceRecordOutput struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceItemID string `json:"source_item_id"`
	OriginalURL  string `json:"original_url"`
	Title        string `json:"title"`
	PublishedAt  string `json:"published_at"`
	RetrievedAt  string `json:"retrieved_at"`
}

type dailyBriefInputOutput struct {
	Region            calendar.Region                `json:"region"`
	PublicationWindow dailyBriefWindowOutput         `json:"publication_window"`
	EventWindow       dailyBriefWindowOutput         `json:"event_window"`
	SourceRecords     []dailyBriefSourceRecordOutput `json:"source_records"`
	UpcomingEvents    []upcomingEventOutput          `json:"upcoming_events"`
}

func runDailyBriefInput(
	ctx context.Context,
	sourceRecords recentSourceRecordsRepository,
	events dailyBriefEventsRepository,
	stdout io.Writer,
	query dailyBriefInputQuery,
) error {
	input, err := assembleDailyBriefInput(ctx, sourceRecords, events, query)
	if err != nil {
		return fmt.Errorf("assemble daily brief input: %w", err)
	}

	output := dailyBriefInputOutput{
		Region: input.region,
		PublicationWindow: dailyBriefWindowOutput{
			From: formatOutputTime(input.publicationWindowStart),
			To:   formatOutputTime(input.publicationWindowEnd),
		},
		EventWindow: dailyBriefWindowOutput{
			From: formatOutputTime(input.eventWindowStart),
			To:   formatOutputTime(input.eventWindowEnd),
		},
		SourceRecords:  make([]dailyBriefSourceRecordOutput, 0, len(input.sourceRecords)),
		UpcomingEvents: make([]upcomingEventOutput, 0, len(input.upcomingEvents)),
	}
	for _, record := range input.sourceRecords {
		output.SourceRecords = append(output.SourceRecords, newDailyBriefSourceRecordOutput(record))
	}
	for _, event := range input.upcomingEvents {
		output.UpcomingEvents = append(output.UpcomingEvents, newUpcomingEventOutput(event))
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("encode daily brief input: %w", err)
	}
	return nil
}

func newDailyBriefSourceRecordOutput(record ingestionpostgres.StoredSourceRecord) dailyBriefSourceRecordOutput {
	return dailyBriefSourceRecordOutput{
		ID:           record.ID,
		Source:       record.Source,
		SourceItemID: record.SourceItemID,
		OriginalURL:  record.OriginalURL,
		Title:        record.Title,
		PublishedAt:  formatOutputTime(record.PublishedAt),
		RetrievedAt:  formatOutputTime(record.RetrievedAt),
	}
}
