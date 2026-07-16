package bls

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	atlasuuid "github.com/Yanis897349/atlas/internal/uuid"
)

const (
	// Source is the normalized Bureau of Labor Statistics observation source.
	Source = "bls"
	// APIURL is the official BLS Public Data API time-series endpoint.
	APIURL = "https://api.bls.gov/publicAPI/v2/timeseries/data/"
	// SeriesCPIAllItemsNSA is the CPI-U all-items U.S. city average, not seasonally adjusted.
	SeriesCPIAllItemsNSA Series = "CUUR0000SA0"
	// SeriesTotalNonfarmPayrollSA is total nonfarm payroll employment, seasonally adjusted.
	SeriesTotalNonfarmPayrollSA Series = "CES0000000001"

	defaultRequestBudget = 30 * time.Second
)

// Series identifies one supported BLS time series.
type Series string

// Target binds one supported BLS series to its canonical economic event.
type Target struct {
	EconomicEventID string
	Series          Series
}

// HTTPClient executes BLS API requests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Config controls BLS observation retrieval and provides deterministic test seams.
type Config struct {
	Targets       []Target
	Endpoint      string
	Client        HTTPClient
	Now           func() time.Time
	RequestBudget time.Duration
}

// NewAdapter validates config and returns a BLS observation adapter.
func NewAdapter(config Config) (*Adapter, error) {
	targets, err := normalizeTargets(config.Targets)
	if err != nil {
		return nil, err
	}
	endpoint, err := normalizeEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if config.RequestBudget < 0 {
		return nil, errors.New("BLS request budget must not be negative")
	}

	requestBudget := config.RequestBudget
	if requestBudget == 0 {
		requestBudget = defaultRequestBudget
	}
	client := config.Client
	if client == nil {
		client = &http.Client{
			Timeout: requestBudget,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	return &Adapter{
		targets:       targets,
		endpoint:      endpoint,
		client:        client,
		now:           now,
		requestBudget: requestBudget,
	}, nil
}

func normalizeTargets(targets []Target) ([]Target, error) {
	if len(targets) == 0 {
		return nil, errors.New("at least one BLS observation target is required")
	}
	if len(targets) > intelligence.MaxObservationIngestionLimit {
		return nil, fmt.Errorf(
			"BLS observation targets must not exceed %d",
			intelligence.MaxObservationIngestionLimit,
		)
	}

	normalized := make([]Target, 0, len(targets))
	events := make(map[string]Series, len(targets))
	series := make(map[Series]string, len(targets))
	for index, target := range targets {
		eventID, valid := atlasuuid.Normalize(target.EconomicEventID)
		if !valid {
			return nil, fmt.Errorf("BLS observation target %d economic event ID must be a UUID", index+1)
		}
		target.EconomicEventID = eventID
		target.Series = Series(strings.TrimSpace(string(target.Series)))
		if !supportedSeries(target.Series) {
			return nil, fmt.Errorf("BLS observation target %d has unsupported series %q", index+1, target.Series)
		}
		if existing, exists := events[eventID]; exists {
			if existing != target.Series {
				return nil, fmt.Errorf("economic event %q is bound to conflicting BLS series", eventID)
			}
			continue
		}
		if existing, exists := series[target.Series]; exists {
			return nil, fmt.Errorf(
				"BLS series %q is bound to conflicting economic events %q and %q",
				target.Series,
				existing,
				eventID,
			)
		}

		events[eventID] = target.Series
		series[target.Series] = eventID
		normalized = append(normalized, target)
	}
	return normalized, nil
}

func supportedSeries(series Series) bool {
	return series == SeriesCPIAllItemsNSA || series == SeriesTotalNonfarmPayrollSA
}

func normalizeEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = APIURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("BLS endpoint must be an absolute HTTP(S) URL without credentials or a fragment")
	}
	if parsed.Scheme != "https" && !isLoopbackHost(parsed.Hostname()) {
		return "", errors.New("BLS endpoint must use HTTPS unless it targets a loopback host")
	}
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.TrimSuffix(strings.ToLower(host), ".") == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
