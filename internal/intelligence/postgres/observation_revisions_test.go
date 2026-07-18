package postgres_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryRetrievesObservationRevisionsDeterministically(t *testing.T) {
	pool := openTestPool(t)
	repository, err := intelligencepostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	event := insertEconomicEvent(t, pool, "observation-revision-query")
	otherEvent := insertEconomicEvent(t, pool, "observation-revision-query-other-event")
	base := time.Date(2026, time.July, 17, 14, 0, 0, 0, time.FixedZone("CEST", 2*60*60))

	initialInput := observationFixture(
		event.ID,
		"revision-query",
		base,
		text("3.2%"),
		text("3.1%"),
		nil,
	)
	initial, err := repository.StoreObservation(t.Context(), initialInput, "initial-worker")
	if err != nil {
		t.Fatalf("StoreObservation(initial) error = %v", err)
	}

	citationInput := initialInput
	citationInput.SourceURL = "https://example.com/releases/revision-query-revised"
	citationInput.ObservedAt = base.Add(time.Hour)
	citation, err := repository.StoreObservation(t.Context(), citationInput, "citation-worker")
	if err != nil {
		t.Fatalf("StoreObservation(citation revision) error = %v", err)
	}

	latestInput := citationInput
	latestInput.ObservedAt = base.Add(2 * time.Hour)
	latestInput.Consensus = nil
	latestInput.Previous = text("3.0%")
	latestInput.Actual = text("3.3%")
	latest, err := repository.StoreObservation(t.Context(), latestInput, "latest-worker")
	if err != nil {
		t.Fatalf("StoreObservation(latest revision) error = %v", err)
	}

	distractors := []intelligence.Observation{
		observationFixture(event.ID, "other-identity", base.Add(3*time.Hour), nil, nil, text("1")),
		observationFixture(otherEvent.ID, "revision-query", base.Add(3*time.Hour), nil, nil, text("2")),
		{
			EconomicEventID:     event.ID,
			Source:              "Official-statistics",
			SourceObservationID: "revision-query",
			SourceURL:           "https://example.com/releases/case-sensitive-source",
			ObservedAt:          base.Add(3 * time.Hour),
			Actual:              text("3"),
		},
	}
	for _, distractor := range distractors {
		if _, err := repository.StoreObservation(t.Context(), distractor, "distractor-worker"); err != nil {
			t.Fatalf("StoreObservation(distractor) error = %v", err)
		}
	}

	limited, err := repository.ObservationRevisions(
		t.Context(),
		strings.ToUpper(event.ID),
		" official-statistics ",
		" revision-query ",
		2,
	)
	if err != nil {
		t.Fatalf("ObservationRevisions(limited) error = %v", err)
	}
	wantLimited := []intelligence.StoredObservation{latest, citation}
	if !reflect.DeepEqual(limited, wantLimited) {
		t.Fatalf("ObservationRevisions(limited) = %#v, want %#v", limited, wantLimited)
	}

	all, err := repository.ObservationRevisions(
		t.Context(),
		event.ID,
		"official-statistics",
		"revision-query",
		10,
	)
	if err != nil {
		t.Fatalf("ObservationRevisions(all) error = %v", err)
	}
	wantAll := []intelligence.StoredObservation{latest, citation, initial}
	if !reflect.DeepEqual(all, wantAll) {
		t.Fatalf("ObservationRevisions(all) = %#v, want %#v", all, wantAll)
	}
	for _, revision := range all {
		if revision.SourceURL == "" || revision.CreatedBy == "" || revision.UpdatedBy == "" {
			t.Errorf("revision = %#v, want complete citation and audit metadata", revision)
		}
		assertUTCObservation(t, revision)
	}
}

func TestRepositoryObservationRevisionsHandlesEmptyAndMissingEvents(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := intelligencepostgres.NewRepository(pool)
	event := insertEconomicEvent(t, pool, "observation-revision-empty")

	empty, err := repository.ObservationRevisions(
		t.Context(),
		event.ID,
		"official-statistics",
		"missing-identity",
		10,
	)
	if err != nil {
		t.Fatalf("ObservationRevisions(empty) error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ObservationRevisions(empty) = %#v, want non-nil empty slice", empty)
	}

	missing, err := repository.ObservationRevisions(
		t.Context(),
		"00000000-0000-0000-0000-000000000999",
		"official-statistics",
		"missing-identity",
		10,
	)
	if !errors.Is(err, pgx.ErrNoRows) ||
		!strings.Contains(err.Error(), "lock economic event for observation revision retrieval") ||
		missing != nil {
		t.Errorf("ObservationRevisions(missing) = (%#v, %v), want nil and contextual pgx.ErrNoRows", missing, err)
	}
}
