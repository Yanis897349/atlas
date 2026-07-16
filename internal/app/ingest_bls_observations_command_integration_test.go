package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/database/postgres/postgrestest"
	intelligencebls "github.com/Yanis897349/atlas/internal/intelligence/bls"
)

func TestRunIngestsBLSObservationsIdempotentlyEndToEnd(t *testing.T) {
	database := postgrestest.Open(t)
	if err := Run(t.Context(), []string{"migrate"}, Dependencies{
		Getenv: applicationDatabaseEnv(database.URL),
	}); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	insertBLSObservationEvent(
		t,
		database,
		validBLSCPIEventID,
		"cpi",
		"Consumer Price Index",
		"inflation",
	)
	insertBLSObservationEvent(
		t,
		database,
		validBLSEmploymentEventID,
		"employment",
		"Employment Situation",
		"employment",
	)

	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		providerCalls.Add(1)
		if request.Method != http.MethodPost {
			t.Errorf("request method = %s, want POST", request.Method)
		}
		var payload struct {
			Series    []intelligencebls.Series `json:"seriesid"`
			StartYear string                   `json:"startyear"`
			EndYear   string                   `json:"endyear"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode BLS request: %v", err)
		}
		wantSeries := []intelligencebls.Series{
			intelligencebls.SeriesCPIAllItemsNSA,
			intelligencebls.SeriesTotalNonfarmPayrollSA,
		}
		if !reflect.DeepEqual(payload.Series, wantSeries) {
			t.Errorf("request series = %#v, want CPI-then-employment order %#v", payload.Series, wantSeries)
		}
		if payload.StartYear != "2024" || payload.EndYear != "2026" {
			t.Errorf("request years = (%q, %q), want explicit three-calendar-year window", payload.StartYear, payload.EndYear)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(testBLSObservationResponse))
	}))
	t.Cleanup(server.Close)

	observedAt := time.Date(2026, time.July, 16, 20, 30, 0, 0, time.FixedZone("Paris", 2*60*60))
	stdout := &bytes.Buffer{}
	dependencies := Dependencies{
		Getenv: applicationDatabaseEnv(database.URL),
		BLSObservations: BLSObservationDependencies{
			HTTPClient: server.Client(),
			Endpoint:   server.URL,
			Now:        func() time.Time { return observedAt },
		},
		Stdout: stdout,
	}

	var firstIDs []string
	for iteration := range 2 {
		if err := Run(t.Context(), validBLSObservationArguments(), dependencies); err != nil {
			t.Fatalf("Run(ingest-bls-observations) iteration %d error = %v", iteration+1, err)
		}
		ids := loadBLSObservationIDs(t, database)
		if iteration == 0 {
			firstIDs = ids
		} else if !reflect.DeepEqual(ids, firstIDs) {
			t.Errorf("observation IDs changed across retry: first %#v, second %#v", firstIDs, ids)
		}
	}

	if providerCalls.Load() != 2 {
		t.Errorf("provider calls = %d, want one per ingestion cycle", providerCalls.Load())
	}
	if stdout.String() != "ingested 2 BLS economic event observations\ningested 2 BLS economic event observations\n" {
		t.Errorf("stdout = %q, want deterministic complete counts", stdout.String())
	}

	rows, err := database.Pool.Query(t.Context(), `
SELECT economic_event_id::text, source, source_observation_id, source_url,
       observed_at, consensus_value, previous_value, actual_value, created_by, updated_by
FROM economic_event_observations
ORDER BY economic_event_id
`)
	if err != nil {
		t.Fatalf("query BLS observations: %v", err)
	}
	defer rows.Close()
	type storedObservation struct {
		eventID, source, sourceID, sourceURL string
		observedAt                           time.Time
		consensus, previous, actual          *string
		createdBy, updatedBy                 string
	}
	var observations []storedObservation
	for rows.Next() {
		var observation storedObservation
		if err := rows.Scan(
			&observation.eventID,
			&observation.source,
			&observation.sourceID,
			&observation.sourceURL,
			&observation.observedAt,
			&observation.consensus,
			&observation.previous,
			&observation.actual,
			&observation.createdBy,
			&observation.updatedBy,
		); err != nil {
			t.Fatalf("scan BLS observation: %v", err)
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate BLS observations: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("observation count = %d, want 2", len(observations))
	}
	assertBLSObservation(t, observations[0], validBLSCPIEventID, "CUUR0000SA0:2026-M06", "3.0%", "3.2%")
	assertBLSObservation(t, observations[1], validBLSEmploymentEventID, "CES0000000001:2026-M06", "+50", "+100")
}

func TestRunRejectsMismatchedBLSObservationEventsBeforeProviderAndWriter(t *testing.T) {
	database := postgrestest.Open(t)
	if err := Run(t.Context(), []string{"migrate"}, Dependencies{
		Getenv: applicationDatabaseEnv(database.URL),
	}); err != nil {
		t.Fatalf("Run(migrate) error = %v", err)
	}
	insertBLSObservationEvent(
		t,
		database,
		validBLSCPIEventID,
		"employment",
		"Employment Situation",
		"employment",
	)
	insertBLSObservationEvent(
		t,
		database,
		validBLSEmploymentEventID,
		"cpi",
		"Consumer Price Index",
		"inflation",
	)

	var providerCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		providerCalls.Add(1)
	}))
	t.Cleanup(server.Close)
	stdout := &bytes.Buffer{}
	err := Run(t.Context(), validBLSObservationArguments(), Dependencies{
		Getenv: applicationDatabaseEnv(database.URL),
		BLSObservations: BLSObservationDependencies{
			HTTPClient: server.Client(),
			Endpoint:   server.URL,
		},
		Stdout: stdout,
	})
	if err == nil || !strings.Contains(err.Error(), "after 0 processed observations") ||
		!strings.Contains(err.Error(), "must be the canonical BLS Consumer Price Index release") {
		t.Fatalf("Run(ingest-bls-observations) error = %v, want canonical event validation", err)
	}
	if providerCalls.Load() != 0 || stdout.Len() != 0 {
		t.Errorf("provider calls = %d, stdout = %q; want no side effects", providerCalls.Load(), stdout.String())
	}
	var count int
	if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM economic_event_observations`).Scan(&count); err != nil {
		t.Fatalf("count BLS observations: %v", err)
	}
	if count != 0 {
		t.Errorf("observation count = %d, want no persistence", count)
	}
}

func insertBLSObservationEvent(
	t *testing.T,
	database postgrestest.Database,
	id, externalID, name, eventType string,
) {
	t.Helper()
	_, err := database.Pool.Exec(t.Context(), `
INSERT INTO economic_events (
    id, source, external_event_id, name, region, event_type, scheduled_at,
    source_url, retrieved_at, created_by, updated_by
)
VALUES ($1, 'bls', $2, $3, 'united_states', $4, '2026-07-16T12:30:00Z',
        'https://www.bls.gov/schedule/', '2026-07-16T10:00:00Z',
        'atlas-bls-calendar-ingestion', 'atlas-bls-calendar-ingestion')
`, id, externalID, name, eventType)
	if err != nil {
		t.Fatalf("insert BLS economic event: %v", err)
	}
}

func loadBLSObservationIDs(t *testing.T, database postgrestest.Database) []string {
	t.Helper()
	rows, err := database.Pool.Query(t.Context(), `
SELECT id::text
FROM economic_event_observations
ORDER BY economic_event_id
`)
	if err != nil {
		t.Fatalf("query BLS observation IDs: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan BLS observation ID: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate BLS observation IDs: %v", err)
	}
	return ids
}

func assertBLSObservation(
	t *testing.T,
	observation struct {
		eventID, source, sourceID, sourceURL string
		observedAt                           time.Time
		consensus, previous, actual          *string
		createdBy, updatedBy                 string
	},
	eventID, sourceID, previous, actual string,
) {
	t.Helper()
	wantObservedAt := time.Date(2026, time.July, 16, 18, 30, 0, 0, time.UTC)
	if observation.eventID != eventID || observation.source != intelligencebls.Source ||
		observation.sourceID != sourceID ||
		observation.sourceURL != "https://data.bls.gov/timeseries/"+strings.SplitN(sourceID, ":", 2)[0] ||
		!observation.observedAt.Equal(wantObservedAt) || observation.consensus != nil ||
		observation.previous == nil || *observation.previous != previous ||
		observation.actual == nil || *observation.actual != actual ||
		observation.createdBy != "atlas-bls-observation-ingestion" ||
		observation.updatedBy != "atlas-bls-observation-ingestion" {
		t.Errorf("observation = %#v, want complete normalized BLS snapshot", observation)
	}
}

const (
	validBLSCPIEventID         = "00000000-0000-0000-0000-000000000091"
	validBLSEmploymentEventID  = "00000000-0000-0000-0000-000000000092"
	testBLSObservationResponse = `{
  "status": "REQUEST_SUCCEEDED",
  "responseTime": 10,
  "message": [],
  "Results": {"series": [
    {"seriesID": "CES0000000001", "data": [
      {"year": "2026", "period": "M06", "periodName": "June", "value": "159000"},
      {"year": "2026", "period": "M05", "periodName": "May", "value": "158900"},
      {"year": "2026", "period": "M04", "periodName": "April", "value": "158850"}
    ]},
    {"seriesID": "CUUR0000SA0", "data": [
      {"year": "2026", "period": "M06", "periodName": "June", "value": "321.500"},
      {"year": "2026", "period": "M05", "periodName": "May", "value": "320.800"},
      {"year": "2026", "period": "M04", "periodName": "April", "value": "319.900"},
      {"year": "2026", "period": "M03", "periodName": "March", "value": "319.000"},
      {"year": "2026", "period": "M02", "periodName": "February", "value": "318.000"},
      {"year": "2026", "period": "M01", "periodName": "January", "value": "317.000"},
      {"year": "2025", "period": "M12", "periodName": "December", "value": "316.500"},
      {"year": "2025", "period": "M11", "periodName": "November", "value": "316.000"},
      {"year": "2025", "period": "M10", "periodName": "October", "value": "315.000"},
      {"year": "2025", "period": "M09", "periodName": "September", "value": "314.000"},
      {"year": "2025", "period": "M08", "periodName": "August", "value": "313.500"},
      {"year": "2025", "period": "M07", "periodName": "July", "value": "312.500"},
      {"year": "2025", "period": "M06", "periodName": "June", "value": "311.500"},
      {"year": "2025", "period": "M05", "periodName": "May", "value": "311.500"}
    ]}
  ]}
}`
)
