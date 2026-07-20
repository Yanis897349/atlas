package app

import (
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
	ingestionpostgres "github.com/Yanis897349/atlas/internal/ingestion/postgres"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/Yanis897349/atlas/internal/search"
	searchpostgres "github.com/Yanis897349/atlas/internal/search/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type economicEventContextObservationFixture struct {
	stored           map[string]intelligence.StoredObservation
	latestInitial    intelligence.StoredObservation
	latestRevision   intelligence.StoredObservation
	officialInitial  intelligence.StoredObservation
	officialCitation intelligence.StoredObservation
	officialLatest   intelligence.StoredObservation
	consensus        string
	previous         string
	actual           string
	revisedActual    string
}

func storeEconomicEventContextObservations(
	t *testing.T,
	pool *pgxpool.Pool,
	eventID string,
	windowEnd time.Time,
) economicEventContextObservationFixture {
	t.Helper()
	repository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository(observations) error = %v", err)
	}
	consensus, previous, actual := "3.1%", "3.0%", "3.3%"
	belowActual, inLineActual := "3.0%", "3.10%"
	inputs := []intelligence.Observation{
		{
			EconomicEventID: eventID, Source: "excluded-statistics",
			SourceObservationID: "cpi-excluded-2026-07",
			SourceURL:           "https://example.com/releases/cpi-excluded-2026-07",
			ObservedAt:          windowEnd.Add(30 * time.Minute), Consensus: &consensus,
		},
		{
			EconomicEventID: eventID, Source: "oldest-statistics",
			SourceObservationID: "cpi-oldest-2026-07",
			SourceURL:           "https://example.com/releases/cpi-oldest-2026-07",
			ObservedAt:          windowEnd.Add(time.Hour), Consensus: &consensus,
		},
		{
			EconomicEventID: eventID, Source: "below-statistics",
			SourceObservationID: "cpi-below-2026-07",
			SourceURL:           "https://example.com/releases/cpi-below-2026-07",
			ObservedAt:          windowEnd.Add(75 * time.Minute), Consensus: &consensus, Actual: &belowActual,
		},
		{
			EconomicEventID: eventID, Source: "inline-statistics",
			SourceObservationID: "cpi-inline-2026-07",
			SourceURL:           "https://example.com/releases/cpi-inline-2026-07",
			ObservedAt:          windowEnd.Add(90 * time.Minute), Consensus: &consensus, Actual: &inLineActual,
		},
		{
			EconomicEventID: eventID, Source: "official-statistics",
			SourceObservationID: "cpi-2026-07",
			SourceURL:           "https://example.com/releases/cpi-2026-07",
			ObservedAt:          windowEnd.Add(2 * time.Hour), Consensus: &consensus, Previous: &previous, Actual: &actual,
		},
		{
			EconomicEventID: eventID, Source: "latest-statistics",
			SourceObservationID: "cpi-latest-2026-07",
			SourceURL:           "https://example.com/releases/cpi-latest-2026-07",
			ObservedAt:          windowEnd.Add(3 * time.Hour), Consensus: &consensus, Previous: &previous, Actual: &actual,
		},
	}
	stored := make(map[string]intelligence.StoredObservation, len(inputs))
	for _, input := range inputs {
		observation, persistErr := repository.StoreObservation(t.Context(), input, "observation-ingestion")
		if persistErr != nil {
			t.Fatalf("StoreObservation(%q) error = %v", input.SourceObservationID, persistErr)
		}
		stored[input.SourceObservationID] = observation
	}

	officialInitial := stored["cpi-2026-07"]
	officialCitationInput := officialInitial.Observation
	officialCitationInput.SourceURL = "https://example.com/releases/cpi-2026-07-corrected"
	officialCitationInput.ObservedAt = windowEnd.Add(2*time.Hour + 20*time.Minute)
	officialCitation, err := repository.StoreObservation(
		t.Context(), officialCitationInput, "observation-citation-correction",
	)
	if err != nil {
		t.Fatalf("StoreObservation(official citation revision) error = %v", err)
	}
	officialLatestInput := officialCitationInput
	officialLatestInput.ObservedAt = windowEnd.Add(2*time.Hour + 40*time.Minute)
	officialLatestInput.Consensus = nil
	revisedActual := "3.5%"
	officialLatestInput.Actual = &revisedActual
	officialLatest, err := repository.StoreObservation(
		t.Context(), officialLatestInput, "observation-value-correction",
	)
	if err != nil {
		t.Fatalf("StoreObservation(official latest revision) error = %v", err)
	}
	stored["cpi-2026-07"] = officialLatest

	latestInitial := stored["cpi-latest-2026-07"]
	latestRevisionInput := latestInitial.Observation
	latestRevisionInput.SourceURL = "https://example.com/releases/cpi-latest-2026-07-corrected"
	latestRevisionInput.ObservedAt = windowEnd.Add(3*time.Hour + 20*time.Minute)
	latestRevision, err := repository.StoreObservation(
		t.Context(), latestRevisionInput, "latest-observation-correction",
	)
	if err != nil {
		t.Fatalf("StoreObservation(latest identity revision) error = %v", err)
	}
	stored["cpi-latest-2026-07"] = latestRevision

	return economicEventContextObservationFixture{
		stored: stored, latestInitial: latestInitial, latestRevision: latestRevision,
		officialInitial: officialInitial, officialCitation: officialCitation, officialLatest: officialLatest,
		consensus: consensus, previous: previous, actual: actual, revisedActual: revisedActual,
	}
}

func storeEconomicEventContextSourceRecords(
	t *testing.T,
	pool *pgxpool.Pool,
	windowStart time.Time,
	windowEnd time.Time,
) map[string]ingestion.StoredSourceRecord {
	t.Helper()
	sourceRepository, err := ingestionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository(source records) error = %v", err)
	}
	type sourceFixture struct {
		itemID      string
		publishedAt time.Time
		vector      []float32
		model       string
	}
	fixtures := []sourceFixture{
		{itemID: "before", publishedAt: windowStart.Add(-time.Microsecond), vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "start", publishedAt: windowStart, vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "middle-a", publishedAt: windowStart.Add(time.Hour), vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "middle-b", publishedAt: windowStart.Add(2 * time.Hour), vector: []float32{1, 1}, model: "embedding-model"},
		{itemID: "end", publishedAt: windowEnd, vector: []float32{0, 1}, model: "embedding-model"},
		{itemID: "after", publishedAt: windowEnd.Add(time.Microsecond), vector: []float32{1, 0}, model: "embedding-model"},
		{itemID: "other-model", publishedAt: windowStart.Add(3 * time.Hour), vector: []float32{1, 0}, model: "other-model"},
		{itemID: "other-dimension", publishedAt: windowStart.Add(3 * time.Hour), vector: []float32{1, 0, 0}, model: "embedding-model"},
	}
	records := make(map[string]ingestion.StoredSourceRecord, len(fixtures))
	embeddings := make([]search.SourceRecordEmbedding, 0, len(fixtures))
	for _, fixture := range fixtures {
		record, persistErr := sourceRepository.UpsertSourceRecord(t.Context(), ingestion.SourceRecord{
			Source:       "test-publisher",
			SourceItemID: fixture.itemID,
			OriginalURL:  "https://example.com/news/" + fixture.itemID,
			Title:        "Story " + fixture.itemID,
			PublishedAt:  fixture.publishedAt,
			RetrievedAt:  fixture.publishedAt.Add(time.Minute),
		}, "rss-ingestion")
		if persistErr != nil {
			t.Fatalf("UpsertSourceRecord(%q) error = %v", fixture.itemID, persistErr)
		}
		records[fixture.itemID] = record
		embeddings = append(embeddings, search.SourceRecordEmbedding{
			SourceRecordID: record.ID, Provider: "openai", Model: fixture.model, Vector: fixture.vector,
		})
	}
	embeddingRepository, err := searchpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository(embeddings) error = %v", err)
	}
	for index, batch := range [][]search.SourceRecordEmbedding{embeddings[:len(embeddings)-1], embeddings[len(embeddings)-1:]} {
		if err := embeddingRepository.PersistSourceRecordEmbeddings(t.Context(), batch, "search-indexer"); err != nil {
			t.Fatalf("PersistSourceRecordEmbeddings(batch %d) error = %v", index, err)
		}
	}
	return records
}
