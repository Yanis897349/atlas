package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	"github.com/Yanis897349/atlas/internal/ingestion"
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
	sourceRecords dailybrief.SourceRecords,
	events dailybrief.Events,
	stdout io.Writer,
	query dailybrief.InputQuery,
) error {
	input, err := dailybrief.AssembleInput(ctx, sourceRecords, events, query)
	if err != nil {
		return fmt.Errorf("assemble daily brief input: %w", err)
	}

	output := dailyBriefInputOutput{
		Region: input.Region,
		PublicationWindow: dailyBriefWindowOutput{
			From: formatOutputTime(input.PublicationWindowStart),
			To:   formatOutputTime(input.PublicationWindowEnd),
		},
		EventWindow: dailyBriefWindowOutput{
			From: formatOutputTime(input.EventWindowStart),
			To:   formatOutputTime(input.EventWindowEnd),
		},
		SourceRecords:  make([]dailyBriefSourceRecordOutput, 0, len(input.SourceRecords)),
		UpcomingEvents: make([]upcomingEventOutput, 0, len(input.UpcomingEvents)),
	}
	for _, record := range input.SourceRecords {
		output.SourceRecords = append(output.SourceRecords, newDailyBriefSourceRecordOutput(record))
	}
	for _, event := range input.UpcomingEvents {
		output.UpcomingEvents = append(output.UpcomingEvents, newUpcomingEventOutput(event))
	}

	return encodeCommandJSON(stdout, "daily brief input", output)
}

func newDailyBriefSourceRecordOutput(record ingestion.StoredSourceRecord) dailyBriefSourceRecordOutput {
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
