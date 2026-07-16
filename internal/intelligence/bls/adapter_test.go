package bls_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	"github.com/Yanis897349/atlas/internal/intelligence/bls"
)

const (
	cpiEventID        = "00000000-0000-0000-0000-000000000001"
	employmentEventID = "00000000-0000-0000-0000-000000000002"
)

func TestAdapterFetchObservationsNormalizesSupportedSeries(t *testing.T) {
	body := fixtureContents(t, "valid.json")
	var requestSeries []bls.Series
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", request.Method)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if got := request.Header.Get("User-Agent"); got != "Atlas (+https://github.com/Yanis897349/atlas)" {
			t.Errorf("User-Agent = %q, want Atlas project identity", got)
		}
		var payload struct {
			Series []bls.Series `json:"seriesid"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requestSeries = payload.Series
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write(body)
	}))
	defer server.Close()

	observedAt := time.Date(2026, time.July, 16, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	adapter := newAdapter(t, bls.Config{
		Targets: []bls.Target{
			{EconomicEventID: employmentEventID, Series: bls.SeriesTotalNonfarmPayrollSA},
			{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA},
		},
		Endpoint: server.URL,
		Now:      func() time.Time { return observedAt },
	})

	observations, err := adapter.FetchObservations(t.Context(), 2)
	if err != nil {
		t.Fatalf("FetchObservations() error = %v", err)
	}
	wantSeries := []bls.Series{bls.SeriesTotalNonfarmPayrollSA, bls.SeriesCPIAllItemsNSA}
	if !reflect.DeepEqual(requestSeries, wantSeries) {
		t.Errorf("requested series = %#v, want %#v", requestSeries, wantSeries)
	}
	if len(observations) != 2 {
		t.Fatalf("FetchObservations() returned %d observations, want 2", len(observations))
	}
	assertObservation(t, observations[0], intelligence.Observation{
		EconomicEventID:     employmentEventID,
		Source:              bls.Source,
		SourceObservationID: "CES0000000001:2026-M06",
		SourceURL:           "https://data.bls.gov/timeseries/CES0000000001",
		ObservedAt:          observedAt.UTC(),
		Previous:            stringPointer("158900"),
		Actual:              stringPointer("159000"),
	})
	assertObservation(t, observations[1], intelligence.Observation{
		EconomicEventID:     cpiEventID,
		Source:              bls.Source,
		SourceObservationID: "CUUR0000SA0:2026-M06",
		SourceURL:           "https://data.bls.gov/timeseries/CUUR0000SA0",
		ObservedAt:          observedAt.UTC(),
		Previous:            stringPointer("320.800"),
		Actual:              stringPointer("321.500"),
	})
}

func TestAdapterFetchObservationsAppliesLimitAfterTargetDeduplication(t *testing.T) {
	client := &recordingClient{response: jsonResponse(`{
		"status":"REQUEST_SUCCEEDED","message":[],"Results":{"series":[{
			"seriesID":"CUUR0000SA0","data":[{"year":"2026","period":"M06","value":"321.500"}]
		}]}}
	`)}
	adapter := newAdapter(t, bls.Config{
		Targets: []bls.Target{
			{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA},
			{EconomicEventID: compactUUID(cpiEventID), Series: bls.SeriesCPIAllItemsNSA},
			{EconomicEventID: employmentEventID, Series: bls.SeriesTotalNonfarmPayrollSA},
		},
		Client: client,
	})

	observations, err := adapter.FetchObservations(t.Context(), 1)
	if err != nil {
		t.Fatalf("FetchObservations() error = %v", err)
	}
	if len(observations) != 1 || observations[0].EconomicEventID != cpiEventID || observations[0].Previous != nil {
		t.Fatalf("FetchObservations() = %#v, want one CPI observation without previous value", observations)
	}
	if !bytes.Contains(client.requestBody, []byte(`"seriesid":["CUUR0000SA0"]`)) {
		t.Errorf("request body = %s, want one deduplicated CPI series", client.requestBody)
	}
	if !bodyClosed(client.response.Body) {
		t.Error("response body was not closed")
	}
}

func TestNewAdapterValidatesConfiguration(t *testing.T) {
	valid := bls.Target{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA}
	tooMany := make([]bls.Target, intelligence.MaxObservationIngestionLimit+1)
	for index := range tooMany {
		tooMany[index] = valid
	}
	tests := []struct {
		name     string
		config   bls.Config
		contains string
	}{
		{name: "missing targets", config: bls.Config{}, contains: "at least one"},
		{name: "too many targets", config: bls.Config{Targets: tooMany}, contains: "must not exceed"},
		{name: "invalid UUID", config: bls.Config{Targets: []bls.Target{{EconomicEventID: "invalid", Series: bls.SeriesCPIAllItemsNSA}}}, contains: "must be a UUID"},
		{name: "unsupported series", config: bls.Config{Targets: []bls.Target{{EconomicEventID: cpiEventID, Series: "OTHER"}}}, contains: "unsupported series"},
		{name: "event bound to two series", config: bls.Config{Targets: []bls.Target{valid, {EconomicEventID: cpiEventID, Series: bls.SeriesTotalNonfarmPayrollSA}}}, contains: "conflicting BLS series"},
		{name: "series bound to two events", config: bls.Config{Targets: []bls.Target{valid, {EconomicEventID: employmentEventID, Series: bls.SeriesCPIAllItemsNSA}}}, contains: "conflicting economic events"},
		{name: "relative endpoint", config: bls.Config{Targets: []bls.Target{valid}, Endpoint: "/api"}, contains: "absolute HTTP(S)"},
		{name: "insecure endpoint", config: bls.Config{Targets: []bls.Target{valid}, Endpoint: "http://example.com/api"}, contains: "must use HTTPS"},
		{name: "endpoint credentials", config: bls.Config{Targets: []bls.Target{valid}, Endpoint: "https://user@example.com/api"}, contains: "without credentials"},
		{name: "negative budget", config: bls.Config{Targets: []bls.Target{valid}, RequestBudget: -time.Second}, contains: "must not be negative"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, err := bls.NewAdapter(test.config)
			if adapter != nil || err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("NewAdapter() = (%#v, %v), want nil and error containing %q", adapter, err, test.contains)
			}
		})
	}
}

func TestAdapterFetchObservationsRejectsInvalidLimitsBeforeHTTP(t *testing.T) {
	adapter := newAdapter(t, bls.Config{
		Targets: []bls.Target{{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA}},
		Client:  panicClient{},
	})
	for _, limit := range []int{0, -1, intelligence.MaxObservationIngestionLimit + 1} {
		if observations, err := adapter.FetchObservations(t.Context(), limit); observations != nil || err == nil ||
			!strings.Contains(err.Error(), "limit must be between") {
			t.Errorf("FetchObservations(limit %d) = (%#v, %v), want validation error", limit, observations, err)
		}
	}
}

func TestAdapterFetchObservationsRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		targets  []bls.Target
		contains string
	}{
		{name: "malformed JSON", body: `{`, contains: "decode BLS API response"},
		{name: "provider failure", body: `{"status":"REQUEST_FAILED","message":[" invalid\nrequest "]}`, contains: "REQUEST_FAILED: invalid request"},
		{name: "missing results", body: `{"status":"REQUEST_SUCCEEDED","message":[]}`, contains: "results are required"},
		{name: "unexpected series", body: successfulSeries("OTHER", validData("1")), contains: "unexpected ID"},
		{name: "missing requested series", body: `{"status":"REQUEST_SUCCEEDED","Results":{"series":[]}}`, contains: "missing requested series"},
		{name: "data-less series", body: successfulSeries("CUUR0000SA0", ``), contains: "at least one monthly"},
		{name: "invalid year", body: successfulSeries("CUUR0000SA0", `{"year":"26","period":"M06","value":"1"}`), contains: "year must contain four digits"},
		{name: "invalid period", body: successfulSeries("CUUR0000SA0", `{"year":"2026","period":"M13","value":"1"}`), contains: "period must be between"},
		{name: "blank value", body: successfulSeries("CUUR0000SA0", `{"year":"2026","period":"M06","value":" "}`), contains: "value must not be blank"},
		{name: "conflicting period", body: successfulSeries("CUUR0000SA0", validData("1")+`,`+validData("2")), contains: "conflicting values"},
		{name: "conflicting repeated series", body: `{"status":"REQUEST_SUCCEEDED","Results":{"series":[` + seriesJSON("CUUR0000SA0", validData("1")) + `,` + seriesJSON("CUUR0000SA0", validData("2")) + `]}}`, contains: "conflicting series"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			targets := test.targets
			if len(targets) == 0 {
				targets = []bls.Target{{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA}}
			}
			adapter := newAdapter(t, bls.Config{Targets: targets, Client: &recordingClient{response: jsonResponse(test.body)}})
			observations, err := adapter.FetchObservations(t.Context(), 1)
			if observations != nil || err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("FetchObservations() = (%#v, %v), want error containing %q", observations, err, test.contains)
			}
		})
	}
}

func TestAdapterFetchObservationsBoundsHTTPAndClosesBodies(t *testing.T) {
	transportErr := errors.New("transport failed")
	readErr := errors.New("read failed")
	tests := []struct {
		name     string
		client   bls.HTTPClient
		contains string
		is       error
	}{
		{name: "transport failure", client: &recordingClient{err: transportErr, response: jsonResponse(`{}`)}, contains: "send BLS API request", is: transportErr},
		{name: "nil response", client: &recordingClient{}, contains: "returned no response"},
		{name: "nil body", client: &recordingClient{response: &http.Response{StatusCode: http.StatusOK}}, contains: "response body is nil"},
		{name: "read failure", client: &recordingClient{response: &http.Response{StatusCode: http.StatusOK, Body: &errorBody{err: readErr}}}, contains: "read BLS API response", is: readErr},
		{name: "oversized response", client: &recordingClient{response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(make([]byte, (1<<20)+1)))}}, contains: "body exceeds"},
		{name: "provider HTTP failure", client: &recordingClient{response: &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(`private provider details`))}}, contains: "status 429"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newAdapter(t, bls.Config{
				Targets: []bls.Target{{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA}},
				Client:  test.client,
			})
			observations, err := adapter.FetchObservations(t.Context(), 1)
			if observations != nil || err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("FetchObservations() = (%#v, %v), want error containing %q", observations, err, test.contains)
			}
			if test.is != nil && !errors.Is(err, test.is) {
				t.Errorf("FetchObservations() error = %v, want errors.Is(%v)", err, test.is)
			}
			if client, ok := test.client.(*recordingClient); ok && client.response != nil && client.response.Body != nil && !bodyClosed(client.response.Body) {
				t.Errorf("response body was not closed")
			}
		})
	}
}

func TestAdapterFetchObservationsPreservesCancellationAndTimeout(t *testing.T) {
	t.Run("pre-canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		adapter := newAdapter(t, bls.Config{
			Targets: []bls.Target{{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA}},
			Client:  panicClient{},
		})
		if observations, err := adapter.FetchObservations(ctx, 1); observations != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("FetchObservations() = (%#v, %v), want context cancellation", observations, err)
		}
	})

	t.Run("request timeout", func(t *testing.T) {
		adapter := newAdapter(t, bls.Config{
			Targets:       []bls.Target{{EconomicEventID: cpiEventID, Series: bls.SeriesCPIAllItemsNSA}},
			Client:        blockingClient{},
			RequestBudget: time.Millisecond,
		})
		if observations, err := adapter.FetchObservations(t.Context(), 1); observations != nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("FetchObservations() = (%#v, %v), want deadline exceeded", observations, err)
		}
	})
}

func assertObservation(t *testing.T, got, want intelligence.Observation) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("observation = %#v, want %#v", got, want)
	}
}

func newAdapter(t *testing.T, config bls.Config) *bls.Adapter {
	t.Helper()
	if config.Endpoint == "" {
		config.Endpoint = "https://example.com/bls"
	}
	adapter, err := bls.NewAdapter(config)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	return adapter
}

func fixtureContents(t *testing.T, name string) []byte {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return contents
}

func compactUUID(value string) string {
	return strings.ReplaceAll(value, "-", "")
}

func stringPointer(value string) *string {
	return &value
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: &trackingBody{Reader: strings.NewReader(body)}}
}

func successfulSeries(seriesID, data string) string {
	return `{"status":"REQUEST_SUCCEEDED","Results":{"series":[` + seriesJSON(seriesID, data) + `]}}`
}

func seriesJSON(seriesID, data string) string {
	return fmt.Sprintf(`{"seriesID":%q,"data":[%s]}`, seriesID, data)
}

func validData(value string) string {
	return fmt.Sprintf(`{"year":"2026","period":"M06","value":%q}`, value)
}

type recordingClient struct {
	response    *http.Response
	err         error
	requestBody []byte
}

func (client *recordingClient) Do(request *http.Request) (*http.Response, error) {
	client.requestBody, _ = io.ReadAll(request.Body)
	return client.response, client.err
}

type panicClient struct{}

func (panicClient) Do(*http.Request) (*http.Response, error) {
	panic("unexpected HTTP request")
}

type blockingClient struct{}

func (blockingClient) Do(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}

type trackingBody struct {
	io.Reader
	closed bool
}

func (body *trackingBody) Close() error {
	body.closed = true
	return nil
}

type errorBody struct {
	err    error
	closed bool
}

func (body *errorBody) Read([]byte) (int, error) {
	return 0, body.err
}

func (body *errorBody) Close() error {
	body.closed = true
	return nil
}

func bodyClosed(body io.ReadCloser) bool {
	switch typed := body.(type) {
	case *trackingBody:
		return typed.closed
	case *errorBody:
		return typed.closed
	default:
		return true
	}
}
