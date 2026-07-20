package app

import (
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/intelligence"
)

type economicEventContextIntegrationObservation struct {
	ID                  string                                       `json:"id"`
	EconomicEventID     string                                       `json:"economic_event_id"`
	Source              string                                       `json:"source"`
	SourceObservationID string                                       `json:"source_observation_id"`
	SourceURL           string                                       `json:"source_url"`
	ObservedAt          string                                       `json:"observed_at"`
	Consensus           *string                                      `json:"consensus"`
	Previous            *string                                      `json:"previous"`
	Actual              *string                                      `json:"actual"`
	Surprise            *string                                      `json:"surprise"`
	SurpriseDirection   *intelligence.SurpriseDirection              `json:"surprise_direction"`
	CreatedAt           string                                       `json:"created_at"`
	UpdatedAt           string                                       `json:"updated_at"`
	CreatedBy           string                                       `json:"created_by"`
	UpdatedBy           string                                       `json:"updated_by"`
	Revisions           []economicEventContextIntegrationObservation `json:"revisions"`
	Comparisons         []economicEventContextIntegrationComparison  `json:"comparisons"`
}

type economicEventContextIntegrationComparison struct {
	NewerRevisionID string                                  `json:"newer_revision_id"`
	OlderRevisionID string                                  `json:"older_revision_id"`
	Changes         []economicEventContextIntegrationChange `json:"changes"`
}

type economicEventContextIntegrationChange struct {
	Field    string  `json:"field"`
	OldValue *string `json:"old_value"`
	NewValue *string `json:"new_value"`
	Delta    *string `json:"delta"`
}

type economicEventContextIntegrationOutput struct {
	Event struct {
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
	} `json:"event"`
	PublicationWindowStart string                                       `json:"publication_window_start"`
	PublicationWindowEnd   string                                       `json:"publication_window_end"`
	Observations           []economicEventContextIntegrationObservation `json:"observations"`
	SourceRecords          []struct {
		ID             string  `json:"id"`
		Source         string  `json:"source"`
		SourceItemID   string  `json:"source_item_id"`
		OriginalURL    string  `json:"original_url"`
		Title          string  `json:"title"`
		PublishedAt    string  `json:"published_at"`
		RetrievedAt    string  `json:"retrieved_at"`
		CreatedAt      string  `json:"created_at"`
		UpdatedAt      string  `json:"updated_at"`
		CreatedBy      string  `json:"created_by"`
		UpdatedBy      string  `json:"updated_by"`
		Provider       string  `json:"provider"`
		Model          string  `json:"model"`
		CosineDistance float64 `json:"cosine_distance"`
	} `json:"source_records"`
}

type economicEventContextIntegrationWant struct {
	rawOutput        []byte
	event            calendar.StoredEvent
	windowStart      time.Time
	windowEnd        time.Time
	observations     map[string]intelligence.StoredObservation
	latestInitial    intelligence.StoredObservation
	latestRevision   intelligence.StoredObservation
	officialInitial  intelligence.StoredObservation
	officialCitation intelligence.StoredObservation
	officialLatest   intelligence.StoredObservation
	consensus        string
	previous         string
	actual           string
	revisedActual    string
	sourceRecords    map[string]ingestion.StoredSourceRecord
}
