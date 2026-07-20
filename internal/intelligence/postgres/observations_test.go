package postgres_test

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryRetrievesObservationsDeterministically(t *testing.T) {
	pool := openTestPool(t)
	repository, _ := intelligencepostgres.NewRepository(pool)
	event := insertEconomicEvent(t, pool, "observation-query")
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	inputs := []intelligence.Observation{
		observationFixture(event.ID, "older", base, nil, nil, text("2.9%")),
		observationFixture(event.ID, "latest-b", base.Add(time.Hour), text("3.1%"), nil, nil),
		observationFixture(event.ID, "latest-a", base.Add(time.Hour), nil, text("3.0%"), text("3.2%")),
	}
	stored := make(map[string]intelligence.StoredObservation, len(inputs))
	for _, input := range inputs {
		result, err := repository.StoreObservation(t.Context(), input, "observation-ingestion")
		if err != nil {
			t.Fatalf("StoreObservation(%q) error = %v", input.SourceObservationID, err)
		}
		stored[input.SourceObservationID] = result
	}
	revisedOlder := inputs[0]
	revisedOlder.ObservedAt = base.Add(2 * time.Hour)
	revisedOlder.Actual = text("3.0%")
	latestOlder, err := repository.StoreObservation(t.Context(), revisedOlder, "revision-worker")
	if err != nil {
		t.Fatalf("StoreObservation(revised older) error = %v", err)
	}

	latestIDs := []string{stored["latest-a"].ID, stored["latest-b"].ID}
	sort.Strings(latestIDs)
	got, err := repository.EventObservations(t.Context(), strings.ToUpper(event.ID), 2)
	if err != nil {
		t.Fatalf("EventObservations() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != latestOlder.ID || got[1].ID != latestIDs[0] {
		t.Fatalf("EventObservations() = %#v, want latest revision then smallest tied UUID", got)
	}
	for _, observation := range got {
		if observation.Source == "" || observation.SourceURL == "" || observation.CreatedBy == "" {
			t.Errorf("observation = %#v, want complete source and audit metadata", observation)
		}
		assertUTCObservation(t, observation)
	}

	all, err := repository.EventObservations(t.Context(), event.ID, 3)
	if err != nil {
		t.Fatalf("EventObservations(all) error = %v", err)
	}
	if len(all) != 3 || all[0].ID != latestOlder.ID ||
		all[1].ID != latestIDs[0] || all[2].ID != latestIDs[1] {
		t.Errorf("EventObservations(all) = %#v, want only latest identity revisions in deterministic order", all)
	}
	for _, observation := range all {
		if observation.ID == stored["older"].ID {
			t.Errorf("EventObservations(all) included superseded revision %q", observation.ID)
		}
	}

	emptyEvent := insertEconomicEvent(t, pool, "observation-empty")
	empty, err := repository.EventObservations(t.Context(), emptyEvent.ID, 10)
	if err != nil {
		t.Fatalf("EventObservations(empty) error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("EventObservations(empty) = %#v, want non-nil empty slice", empty)
	}

	missing, err := repository.EventObservations(
		t.Context(),
		"00000000-0000-0000-0000-000000000999",
		10,
	)
	if !errors.Is(err, pgx.ErrNoRows) || missing != nil {
		t.Errorf("EventObservations(missing) = (%#v, %v), want nil and pgx.ErrNoRows", missing, err)
	}
}
