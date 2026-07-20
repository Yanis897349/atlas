package postgres_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
)

func TestRepositoryStoresImmutableObservationRevisions(t *testing.T) {
	pool := openTestPool(t)
	repository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	event := insertEconomicEvent(t, pool, "observation-store")
	location := time.FixedZone("CEST", 2*60*60)
	initial := intelligence.Observation{
		EconomicEventID:     strings.ToUpper(event.ID),
		Source:              " official-statistics ",
		SourceObservationID: " cpi-2026-06 ",
		SourceURL:           "https://example.com/releases/cpi-2026-06",
		ObservedAt:          time.Date(2026, time.July, 15, 14, 0, 0, 0, location),
		Consensus:           text(" 3.2% "),
		Previous:            text("3.1%"),
	}

	created, err := repository.StoreObservation(t.Context(), initial, " observation-ingestion ")
	if err != nil {
		t.Fatalf("first StoreObservation() error = %v", err)
	}
	if created.EconomicEventID != event.ID || created.Source != "official-statistics" ||
		created.SourceObservationID != "cpi-2026-06" || value(created.Consensus) != "3.2%" ||
		value(created.Previous) != "3.1%" || created.Actual != nil ||
		created.CreatedBy != "observation-ingestion" || created.UpdatedBy != "observation-ingestion" {
		t.Errorf("created observation = %#v, want normalized exact snapshot", created)
	}
	assertUTCObservation(t, created)

	retried, err := repository.StoreObservation(t.Context(), initial, "retry-worker")
	if err != nil {
		t.Fatalf("retry StoreObservation() error = %v", err)
	}
	if !reflect.DeepEqual(retried, created) {
		t.Errorf("retried observation = %#v, want unchanged %#v", retried, created)
	}
	assertObservationCount(t, pool, 1)

	laterUnchanged := initial
	laterUnchanged.ObservedAt = initial.ObservedAt.Add(time.Hour)
	unchanged, err := repository.StoreObservation(t.Context(), laterUnchanged, "later-worker")
	if err != nil {
		t.Fatalf("later unchanged StoreObservation() error = %v", err)
	}
	if !reflect.DeepEqual(unchanged, created) {
		t.Errorf("later unchanged observation = %#v, want original revision %#v", unchanged, created)
	}
	assertObservationCount(t, pool, 1)

	delayedOlder := initial
	delayedOlder.Source = "official-statistics"
	delayedOlder.SourceObservationID = "cpi-2026-06"
	delayedOlder.ObservedAt = initial.ObservedAt.Add(30 * time.Minute)
	delayedOlder.Actual = text("stale")
	unchanged, err = repository.StoreObservation(t.Context(), delayedOlder, "older-worker")
	if err != nil {
		t.Fatalf("delayed older StoreObservation() error = %v", err)
	}
	if !reflect.DeepEqual(unchanged, created) {
		t.Errorf("delayed older observation = %#v, want unchanged %#v", unchanged, created)
	}

	citationChange := initial
	citationChange.SourceURL = "https://example.com/releases/cpi-2026-06-revised"
	citationChange.ObservedAt = initial.ObservedAt.Add(2 * time.Hour)
	citationRevision, err := repository.StoreObservation(t.Context(), citationChange, "citation-worker")
	if err != nil {
		t.Fatalf("citation revision StoreObservation() error = %v", err)
	}
	if citationRevision.ID == created.ID || citationRevision.SourceURL != citationChange.SourceURL ||
		citationRevision.CreatedBy != "citation-worker" || citationRevision.UpdatedBy != "citation-worker" {
		t.Errorf("citation revision = %#v, want distinct immutable revision", citationRevision)
	}
	assertUTCObservation(t, citationRevision)

	newer := citationChange
	newer.Source = "official-statistics"
	newer.SourceObservationID = "cpi-2026-06"
	newer.ObservedAt = initial.ObservedAt.Add(3 * time.Hour)
	newer.Consensus = nil
	newer.Previous = text("3.0%")
	newer.Actual = text("3.3%")
	revised, err := repository.StoreObservation(t.Context(), newer, "refresh-worker")
	if err != nil {
		t.Fatalf("newer StoreObservation() error = %v", err)
	}
	if revised.ID == created.ID || revised.ID == citationRevision.ID {
		t.Errorf("newer observation ID = %q, want a new revision", revised.ID)
	}
	if revised.Consensus != nil || value(revised.Previous) != "3.0%" || value(revised.Actual) != "3.3%" ||
		revised.SourceURL != newer.SourceURL || !revised.ObservedAt.Equal(newer.ObservedAt) ||
		revised.CreatedBy != "refresh-worker" || revised.UpdatedBy != "refresh-worker" {
		t.Errorf("newer observation = %#v, want immutable changed revision", revised)
	}
	assertUTCObservation(t, revised)
	assertObservationCount(t, pool, 3)

	var (
		originalURL, originalPrevious string
		originalConsensus             *string
		originalActual                *string
		originalCreatedBy             string
		originalUpdatedBy             string
	)
	if err := pool.QueryRow(t.Context(), `
SELECT source_url, consensus_value, previous_value, actual_value, created_by, updated_by
FROM economic_event_observations
WHERE id = $1
`, created.ID).Scan(
		&originalURL,
		&originalConsensus,
		&originalPrevious,
		&originalActual,
		&originalCreatedBy,
		&originalUpdatedBy,
	); err != nil {
		t.Fatalf("load original observation revision: %v", err)
	}
	if originalURL != created.SourceURL || value(originalConsensus) != "3.2%" ||
		originalPrevious != "3.1%" || originalActual != nil ||
		originalCreatedBy != "observation-ingestion" || originalUpdatedBy != "observation-ingestion" {
		t.Errorf("original observation revision changed after later revisions")
	}
}

func TestRepositoryComparesObservationTimesAtPostgreSQLPrecision(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := intelligencepostgres.NewRepository(pool)
	event := insertEconomicEvent(t, pool, "observation-timestamp-precision")
	base := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
	initial := observationFixture(
		event.ID,
		"precision",
		base.Add(100*time.Nanosecond),
		nil,
		nil,
		text("3.0%"),
	)
	created, err := repository.StoreObservation(t.Context(), initial, "worker")
	if err != nil {
		t.Fatalf("StoreObservation(initial) error = %v", err)
	}
	if !created.ObservedAt.Equal(base) {
		t.Fatalf("initial observed at = %s, want PostgreSQL precision %s", created.ObservedAt, base)
	}

	equalAtDatabasePrecision := initial
	equalAtDatabasePrecision.ObservedAt = base.Add(900 * time.Nanosecond)
	equalAtDatabasePrecision.Actual = text("3.1%")
	unchanged, err := repository.StoreObservation(t.Context(), equalAtDatabasePrecision, "equal-worker")
	if err != nil {
		t.Fatalf("StoreObservation(equal PostgreSQL timestamp) error = %v", err)
	}
	if !reflect.DeepEqual(unchanged, created) {
		t.Errorf("equal PostgreSQL timestamp observation = %#v, want unchanged %#v", unchanged, created)
	}
	assertObservationCount(t, pool, 1)

	newer := equalAtDatabasePrecision
	newer.ObservedAt = base.Add(time.Microsecond + 100*time.Nanosecond)
	revised, err := repository.StoreObservation(t.Context(), newer, "revision-worker")
	if err != nil {
		t.Fatalf("StoreObservation(newer PostgreSQL timestamp) error = %v", err)
	}
	if revised.ID == created.ID || !revised.ObservedAt.Equal(base.Add(time.Microsecond)) {
		t.Errorf("newer PostgreSQL timestamp revision = %#v, want distinct microsecond revision", revised)
	}
	assertObservationCount(t, pool, 2)
}
