package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	calendarpostgres "github.com/Yanis897349/atlas/internal/calendar/postgres"
	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	"github.com/Yanis897349/atlas/internal/intelligence"
	intelligencepostgres "github.com/Yanis897349/atlas/internal/intelligence/postgres"
	"github.com/jackc/pgx/v5"
)

func TestRunRetrievesObservationRevisionsEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	dependencies := Dependencies{Getenv: func(name string) string {
		if name == "ATLAS_DATABASE_URL" {
			return database.URL
		}
		if strings.HasPrefix(name, "ATLAS_OPENAI_") {
			t.Fatalf("read provider configuration %q for observation revision command", name)
		}
		return ""
	}}
	if err := Run(t.Context(), []string{"migrate"}, dependencies); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}

	eventRepository, err := calendarpostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(events) error = %v", err)
	}
	base := time.Date(2026, time.July, 18, 12, 0, 0, 123456000, time.FixedZone("CEST", 2*60*60))
	event := storeObservationRevisionEvent(t, eventRepository, "revision-command", base)
	otherEvent := storeObservationRevisionEvent(t, eventRepository, "revision-command-other", base.Add(time.Hour))

	observationRepository, err := intelligencepostgres.NewRepository(database.Pool)
	if err != nil {
		t.Fatalf("NewRepository(observations) error = %v", err)
	}
	consensus, previous, actual := "3.1%", "3.0%", "3.3%"
	initialInput := intelligence.Observation{
		EconomicEventID:     event.ID,
		Source:              "official-statistics",
		SourceObservationID: "cpi-2026-07",
		SourceURL:           "https://example.com/releases/cpi-initial",
		ObservedAt:          base,
		Consensus:           &consensus,
		Previous:            &previous,
	}
	initial, err := observationRepository.StoreObservation(t.Context(), initialInput, "initial-worker")
	if err != nil {
		t.Fatalf("StoreObservation(initial) error = %v", err)
	}
	citationInput := initialInput
	citationInput.SourceURL = "https://example.com/releases/cpi-corrected"
	citationInput.ObservedAt = base.Add(time.Hour)
	citation, err := observationRepository.StoreObservation(t.Context(), citationInput, "citation-worker")
	if err != nil {
		t.Fatalf("StoreObservation(citation) error = %v", err)
	}
	latestInput := citationInput
	latestInput.ObservedAt = base.Add(2 * time.Hour)
	latestInput.Consensus = nil
	latestInput.Previous = &previous
	latestInput.Actual = &actual
	latest, err := observationRepository.StoreObservation(t.Context(), latestInput, "latest-worker")
	if err != nil {
		t.Fatalf("StoreObservation(latest) error = %v", err)
	}

	distractors := []intelligence.Observation{
		{
			EconomicEventID: event.ID, Source: "official-statistics", SourceObservationID: "other-identity",
			SourceURL: "https://example.com/releases/other-identity", ObservedAt: base.Add(3 * time.Hour), Actual: &actual,
		},
		{
			EconomicEventID: event.ID, Source: "Official-statistics", SourceObservationID: "cpi-2026-07",
			SourceURL: "https://example.com/releases/case-sensitive-source", ObservedAt: base.Add(3 * time.Hour), Actual: &actual,
		},
		{
			EconomicEventID: otherEvent.ID, Source: "official-statistics", SourceObservationID: "cpi-2026-07",
			SourceURL: "https://example.com/releases/other-event", ObservedAt: base.Add(3 * time.Hour), Actual: &actual,
		},
	}
	for _, distractor := range distractors {
		if _, err := observationRepository.StoreObservation(t.Context(), distractor, "distractor-worker"); err != nil {
			t.Fatalf("StoreObservation(distractor) error = %v", err)
		}
	}

	stdout := &bytes.Buffer{}
	dependencies.Stdout = stdout
	arguments := []string{
		"economic-event-observation-revisions",
		"--event-id", strings.ToUpper(event.ID),
		"--source", " official-statistics ",
		"--source-observation-id", " cpi-2026-07 ",
		"--limit", "2",
	}
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(economic-event-observation-revisions) error = %v", err)
	}

	var output []observationRevisionIntegrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	if len(output) != 2 || output[0].ID != latest.ID || output[1].ID != citation.ID {
		t.Fatalf("output IDs = %#v, want bounded newest revisions [%q %q]", output, latest.ID, citation.ID)
	}
	if output[0].EconomicEventID != event.ID || output[0].Source != "official-statistics" ||
		output[0].SourceObservationID != "cpi-2026-07" || output[0].SourceURL != latest.SourceURL ||
		output[0].ObservedAt != formatOutputTime(latest.ObservedAt) || output[0].Consensus != nil ||
		output[0].Previous == nil || *output[0].Previous != previous ||
		output[0].Actual == nil || *output[0].Actual != actual ||
		output[0].CreatedAt != formatOutputTime(latest.CreatedAt) ||
		output[0].UpdatedAt != formatOutputTime(latest.UpdatedAt) ||
		output[0].CreatedBy != "latest-worker" || output[0].UpdatedBy != "latest-worker" ||
		output[1].SourceURL != citation.SourceURL || output[1].Consensus == nil ||
		*output[1].Consensus != consensus || output[1].Actual != nil ||
		output[1].CreatedBy != "citation-worker" || output[1].UpdatedBy != "citation-worker" {
		t.Errorf("output = %#v, want complete exact nullable values, citations, and UTC audit metadata", output)
	}
	if initial.ID == output[0].ID || initial.ID == output[1].ID {
		t.Errorf("output included initial revision %q beyond limit", initial.ID)
	}

	stdout.Reset()
	arguments[6] = "missing-identity"
	if err := Run(t.Context(), arguments, dependencies); err != nil {
		t.Fatalf("Run(empty observation revisions) error = %v", err)
	}
	output = nil
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode empty command output: %v", err)
	}
	if output == nil || len(output) != 0 || stdout.String() != "[]\n" {
		t.Errorf("empty output = (%#v, %q), want non-nil []", output, stdout.String())
	}

	stdout.Reset()
	arguments[2] = "00000000-0000-0000-0000-000000000999"
	err = Run(t.Context(), arguments, dependencies)
	if !errors.Is(err, pgx.ErrNoRows) ||
		!strings.Contains(err.Error(), "retrieve economic event observation revisions") || stdout.Len() != 0 {
		t.Fatalf("missing event = (%v, %q), want contextual pgx.ErrNoRows without output", err, stdout.String())
	}
}

func storeObservationRevisionEvent(
	t *testing.T,
	repository *calendarpostgres.Repository,
	externalID string,
	retrievedAt time.Time,
) calendar.StoredEvent {
	t.Helper()
	event, err := repository.UpsertEvent(t.Context(), calendar.Event{
		Source:          "official-calendar",
		ExternalEventID: externalID,
		Name:            "Consumer Price Index",
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeInflation,
		ScheduledAt:     retrievedAt.Add(24 * time.Hour),
		SourceURL:       "https://example.com/calendar/" + externalID,
		RetrievedAt:     retrievedAt,
	}, "calendar-ingestion")
	if err != nil {
		t.Fatalf("UpsertEvent(%q) error = %v", externalID, err)
	}
	return event
}

type observationRevisionIntegrationOutput struct {
	ID                  string  `json:"id"`
	EconomicEventID     string  `json:"economic_event_id"`
	Source              string  `json:"source"`
	SourceObservationID string  `json:"source_observation_id"`
	SourceURL           string  `json:"source_url"`
	ObservedAt          string  `json:"observed_at"`
	Consensus           *string `json:"consensus"`
	Previous            *string `json:"previous"`
	Actual              *string `json:"actual"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
	CreatedBy           string  `json:"created_by"`
	UpdatedBy           string  `json:"updated_by"`
}
