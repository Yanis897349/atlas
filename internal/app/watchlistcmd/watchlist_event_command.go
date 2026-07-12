package watchlistcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/watchlist"
)

type watchlistEventOutput struct {
	ID          string                    `json:"id"`
	WatchlistID string                    `json:"watchlist_id"`
	Symbol      string                    `json:"symbol"`
	Event       storedEconomicEventOutput `json:"event"`
	CreatedAt   string                    `json:"created_at"`
	UpdatedAt   string                    `json:"updated_at"`
	CreatedBy   string                    `json:"created_by"`
	UpdatedBy   string                    `json:"updated_by"`
}

type storedEconomicEventOutput struct {
	ID              string             `json:"id"`
	Source          string             `json:"source"`
	ExternalEventID string             `json:"external_event_id"`
	Name            string             `json:"name"`
	Region          calendar.Region    `json:"region"`
	EventType       calendar.EventType `json:"event_type"`
	ScheduledAt     string             `json:"scheduled_at"`
	SourceURL       string             `json:"source_url"`
	RetrievedAt     string             `json:"retrieved_at"`
	CreatedAt       string             `json:"created_at"`
	UpdatedAt       string             `json:"updated_at"`
	CreatedBy       string             `json:"created_by"`
	UpdatedBy       string             `json:"updated_by"`
}

func runLinkWatchlistEvent(
	ctx context.Context,
	repository watchlistEventLinkCreator,
	stdout io.Writer,
	command linkWatchlistEventCommand,
) error {
	stored, err := repository.CreateEventLink(
		ctx, command.watchlistID, command.symbol, command.eventID, command.actor,
	)
	if err != nil {
		return fmt.Errorf("link watchlist event: %w", err)
	}
	return output.EncodeJSON(stdout, "linked watchlist event", newWatchlistEventOutput(stored))
}

func runWatchlistEvents(
	ctx context.Context,
	repository watchlistEventLinkReader,
	stdout io.Writer,
	query watchlistEventsQuery,
) error {
	stored, err := repository.EventLinks(ctx, query.watchlistID, query.symbol, query.limit)
	if err != nil {
		return fmt.Errorf("retrieve watchlist events: %w", err)
	}
	result := make([]watchlistEventOutput, 0, len(stored))
	for _, link := range stored {
		result = append(result, newWatchlistEventOutput(link))
	}
	return output.EncodeJSON(stdout, "watchlist events", result)
}

func newWatchlistEventOutput(link watchlist.StoredEventLink) watchlistEventOutput {
	return watchlistEventOutput{
		ID: link.ID, WatchlistID: link.WatchlistID, Symbol: link.Symbol,
		Event:     newStoredEconomicEventOutput(link.Event),
		CreatedAt: output.FormatTime(link.CreatedAt),
		UpdatedAt: output.FormatTime(link.UpdatedAt),
		CreatedBy: link.CreatedBy, UpdatedBy: link.UpdatedBy,
	}
}

func newStoredEconomicEventOutput(event calendar.StoredEvent) storedEconomicEventOutput {
	return storedEconomicEventOutput{
		ID: event.ID, Source: event.Source, ExternalEventID: event.ExternalEventID,
		Name: event.Name, Region: event.Region, EventType: event.Type,
		ScheduledAt: output.FormatTime(event.ScheduledAt), SourceURL: event.SourceURL,
		RetrievedAt: output.FormatTime(event.RetrievedAt),
		CreatedAt:   output.FormatTime(event.CreatedAt), UpdatedAt: output.FormatTime(event.UpdatedAt),
		CreatedBy: event.CreatedBy, UpdatedBy: event.UpdatedBy,
	}
}
