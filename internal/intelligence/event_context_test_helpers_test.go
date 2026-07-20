package intelligence

import (
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/ingestion"
	"github.com/Yanis897349/atlas/internal/search"
)

const validEventID = "00000000-0000-0000-0000-000000000083"

func validEventContextQuery() EventContextQuery {
	windowStart := time.Date(2026, time.July, 10, 8, 0, 0, 0, time.UTC)
	return EventContextQuery{
		EventID:                  validEventID,
		PublicationWindowStart:   windowStart,
		PublicationWindowEnd:     windowStart.Add(24 * time.Hour),
		SourceRecordLimit:        20,
		ObservationLimit:         10,
		ObservationRevisionLimit: 10,
	}
}

func withEventContextQuery(query EventContextQuery, update func(*EventContextQuery)) EventContextQuery {
	update(&query)
	return query
}

func storedEventFixture(id, name string) calendar.StoredEvent {
	scheduledAt := time.Date(2026, time.July, 14, 12, 30, 0, 0, time.UTC)
	return calendar.StoredEvent{
		ID: id,
		Event: calendar.Event{
			Source:          "official-calendar",
			ExternalEventID: "event-83",
			Name:            name,
			Region:          calendar.RegionUnitedStates,
			Type:            calendar.EventTypeInflation,
			ScheduledAt:     scheduledAt,
			SourceURL:       "https://example.com/calendar/event-83",
			RetrievedAt:     scheduledAt.Add(-time.Hour),
		},
		CreatedAt: scheduledAt.Add(-2 * time.Hour),
		UpdatedAt: scheduledAt.Add(-time.Hour),
		CreatedBy: "calendar-ingestion",
		UpdatedBy: "calendar-refresh",
	}
}

func similarSourceRecordFixture(id, title string, distance float64) search.SimilarSourceRecord {
	publishedAt := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)
	return search.SimilarSourceRecord{
		SourceRecord: ingestion.StoredSourceRecord{
			ID: id,
			SourceRecord: ingestion.SourceRecord{
				Source:       "publisher",
				SourceItemID: "item-" + id,
				OriginalURL:  "https://example.com/news/" + id,
				Title:        title,
				PublishedAt:  publishedAt,
				RetrievedAt:  publishedAt.Add(time.Minute),
			},
			CreatedAt: publishedAt.Add(2 * time.Minute),
			UpdatedAt: publishedAt.Add(3 * time.Minute),
			CreatedBy: "rss-ingestion",
			UpdatedBy: "rss-refresh",
		},
		Provider:       "openai",
		Model:          "embedding-model",
		CosineDistance: distance,
	}
}

func storedObservationFixture(id, eventID, sourceID string, observedAt time.Time) StoredObservation {
	consensus := " 3.2% "
	previous := "3.1%"
	actual := "3.3%"
	return StoredObservation{
		ID: id,
		Observation: Observation{
			EconomicEventID:     eventID,
			Source:              "official-statistics",
			SourceObservationID: sourceID,
			SourceURL:           "https://example.com/releases/" + sourceID,
			ObservedAt:          observedAt,
			Consensus:           &consensus,
			Previous:            &previous,
			Actual:              &actual,
		},
		CreatedAt: observedAt.Add(time.Minute),
		UpdatedAt: observedAt.Add(2 * time.Minute),
		CreatedBy: "observation-ingestion",
		UpdatedBy: "observation-refresh",
	}
}

func validEmbeddingBatch() search.EmbeddingBatch {
	return search.EmbeddingBatch{
		Provider: "provider",
		Model:    "model",
		Embeddings: []search.ProviderEmbedding{{
			SourceRecordID: "semantic-search-query",
			Vector:         []float32{1, 2},
		}},
	}
}

func failureContext(eventErr, observationErr, revisionErr error) string {
	if eventErr != nil {
		return "retrieve economic event"
	}
	if observationErr != nil {
		return "retrieve economic event observations"
	}
	if revisionErr != nil {
		return "retrieve economic event observation revisions"
	}
	return "retrieve economic event source records"
}
