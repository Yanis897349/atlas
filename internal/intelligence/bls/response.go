package bls

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
)

const successfulStatus = "REQUEST_SUCCEEDED"

type apiResponse struct {
	Status   string      `json:"status"`
	Messages []string    `json:"message"`
	Results  *apiResults `json:"Results"`
}

type apiResults struct {
	Series []apiSeries `json:"series"`
}

type apiSeries struct {
	SeriesID string    `json:"seriesID"`
	Data     []apiData `json:"data"`
}

type apiData struct {
	Year   string `json:"year"`
	Period string `json:"period"`
	Value  string `json:"value"`
}

type snapshot struct {
	seriesID string
	year     string
	period   string
	actual   string
	previous *string
}

func normalizeResponse(
	body []byte,
	targets []Target,
	observedAt time.Time,
) ([]intelligence.Observation, error) {
	var response apiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode BLS API response: %w", err)
	}
	if response.Status != successfulStatus {
		return nil, providerStatusError(response.Status, response.Messages)
	}
	if response.Results == nil {
		return nil, errors.New("BLS API response results are required")
	}

	requested := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		requested[string(target.Series)] = struct{}{}
	}
	snapshots := make(map[string]snapshot, len(targets))
	for index, series := range response.Results.Series {
		seriesID := strings.TrimSpace(series.SeriesID)
		if _, exists := requested[seriesID]; !exists {
			return nil, fmt.Errorf("BLS API response series %d has unexpected ID %q", index+1, seriesID)
		}
		normalized, err := normalizeSeries(series)
		if err != nil {
			return nil, fmt.Errorf("normalize BLS API response series %q: %w", seriesID, err)
		}
		if existing, exists := snapshots[seriesID]; exists {
			if !reflect.DeepEqual(existing, normalized) {
				return nil, fmt.Errorf("BLS API response contains conflicting series %q", seriesID)
			}
			continue
		}
		snapshots[seriesID] = normalized
	}

	observations := make([]intelligence.Observation, 0, len(targets))
	for _, target := range targets {
		snapshot, exists := snapshots[string(target.Series)]
		if !exists {
			return nil, fmt.Errorf("BLS API response is missing requested series %q", target.Series)
		}
		actual := snapshot.actual
		observations = append(observations, intelligence.Observation{
			EconomicEventID:     target.EconomicEventID,
			Source:              Source,
			SourceObservationID: snapshot.seriesID + ":" + snapshot.year + "-" + snapshot.period,
			SourceURL:           "https://data.bls.gov/timeseries/" + snapshot.seriesID,
			ObservedAt:          observedAt,
			Previous:            snapshot.previous,
			Actual:              &actual,
		})
	}
	return observations, nil
}

func normalizeSeries(series apiSeries) (snapshot, error) {
	seriesID := strings.TrimSpace(series.SeriesID)
	if seriesID == "" {
		return snapshot{}, errors.New("series ID is required")
	}

	values := make([]apiData, 0, len(series.Data))
	seen := make(map[string]string, len(series.Data))
	for index, data := range series.Data {
		if err := validateData(data); err != nil {
			return snapshot{}, fmt.Errorf("data point %d: %w", index+1, err)
		}
		identity := data.Year + "-" + data.Period
		if value, exists := seen[identity]; exists {
			if value != data.Value {
				return snapshot{}, fmt.Errorf("period %q has conflicting values", identity)
			}
			continue
		}
		seen[identity] = data.Value
		values = append(values, data)
	}
	if len(values) == 0 {
		return snapshot{}, errors.New("at least one monthly data point is required")
	}

	result := snapshot{
		seriesID: seriesID,
		year:     values[0].Year,
		period:   values[0].Period,
		actual:   values[0].Value,
	}
	if len(values) > 1 {
		previous := values[1].Value
		result.previous = &previous
	}
	return result, nil
}

func validateData(data apiData) error {
	if len(data.Year) != 4 {
		return errors.New("year must contain four digits")
	}
	year, err := strconv.Atoi(data.Year)
	if err != nil || year < 1000 {
		return errors.New("year must contain four digits")
	}
	if len(data.Period) != 3 || data.Period[0] != 'M' {
		return errors.New("period must be between M01 and M12")
	}
	month, err := strconv.Atoi(data.Period[1:])
	if err != nil || month < 1 || month > 12 {
		return errors.New("period must be between M01 and M12")
	}
	if strings.TrimSpace(data.Value) == "" {
		return errors.New("value must not be blank")
	}
	return nil
}

func providerStatusError(status string, messages []string) error {
	status = sanitizeProviderValue(status)
	const maxMessages = 4
	parts := make([]string, 0, min(len(messages), maxMessages))
	for _, message := range messages[:min(len(messages), maxMessages)] {
		if message = sanitizeProviderValue(message); message != "" {
			parts = append(parts, message)
		}
	}
	if status == "" {
		status = "unknown status"
	}
	if len(parts) == 0 {
		return fmt.Errorf("BLS API returned %s", status)
	}
	return fmt.Errorf("BLS API returned %s: %s", status, strings.Join(parts, "; "))
}

func sanitizeProviderValue(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maxBytes = 256
	if len(value) > maxBytes {
		return value[:maxBytes] + "..."
	}
	return value
}
