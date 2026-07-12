package app

import "github.com/Yanis897349/atlas/internal/calendar"

type watchlistOutput struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Symbols   []string `json:"symbols"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	CreatedBy string   `json:"created_by"`
	UpdatedBy string   `json:"updated_by"`
}

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
