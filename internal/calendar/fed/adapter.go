// Package fed retrieves and normalizes scheduled Federal Open Market Committee meetings.
package fed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

const (
	// Source is the normalized name of the Federal Reserve calendar source.
	Source = "federal_reserve"
	// CalendarURL is the canonical Federal Reserve FOMC meeting calendar.
	CalendarURL = "https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm"

	defaultRequestBudget = 30 * time.Second
)

// HTTPClient is the subset of http.Client used to retrieve the calendar.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Config controls calendar retrieval and provides deterministic test seams.
type Config struct {
	CalendarURL   string
	Client        HTTPClient
	Now           func() time.Time
	RequestBudget time.Duration
}

// Adapter retrieves and normalizes the Federal Reserve FOMC calendar.
type Adapter struct {
	calendarURL   string
	client        HTTPClient
	now           func() time.Time
	requestBudget time.Duration
}

// NewAdapter validates config and returns a Federal Reserve calendar adapter.
func NewAdapter(config Config) (*Adapter, error) {
	calendarURL := config.CalendarURL
	if calendarURL == "" {
		calendarURL = CalendarURL
	}
	validatedURL, err := validateHTTPURL(calendarURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Federal Reserve calendar URL: %w", err)
	}

	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: defaultRequestBudget}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	if config.RequestBudget < 0 {
		return nil, errors.New("request budget must not be negative")
	}
	requestBudget := config.RequestBudget
	if requestBudget == 0 {
		requestBudget = defaultRequestBudget
	}

	return &Adapter{
		calendarURL:   validatedURL,
		client:        client,
		now:           now,
		requestBudget: requestBudget,
	}, nil
}

// FetchEvents retrieves the configured page and returns regular unique meetings.
func (adapter *Adapter) FetchEvents(ctx context.Context) ([]calendar.Event, error) {
	body, err := adapter.fetchBody(ctx)
	if err != nil {
		return nil, err
	}

	events, err := parseEvents(body, adapter.now().UTC())
	if err != nil {
		return nil, fmt.Errorf("parse Federal Reserve calendar: %w", err)
	}
	return events, nil
}

func validateHTTPURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", errors.New("must be an absolute HTTP(S) URL")
	}
	return parsed.String(), nil
}
