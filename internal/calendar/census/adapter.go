// Package census retrieves and normalizes scheduled U.S. Census Bureau releases.
package census

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehttp"
)

const (
	// Source is the normalized name of the U.S. Census Bureau calendar source.
	Source = "census"
	// CalendarURL is the canonical U.S. Census Bureau economic indicator release schedule.
	CalendarURL = "https://www.census.gov/economic-indicators/calendar-listview.html"

	resource = "Census calendar"
)

// HTTPClient is the subset of http.Client used to retrieve the calendar.
type HTTPClient = sourcehttp.Client

// Config controls calendar retrieval and provides deterministic test seams.
type Config struct {
	CalendarURL   string
	Client        HTTPClient
	Now           func() time.Time
	RequestBudget time.Duration
}

// Adapter retrieves and normalizes the Census economic indicator calendar.
type Adapter struct {
	fetcher *sourcehttp.Fetcher
	now     func() time.Time
}

// NewAdapter validates config and returns a Census calendar adapter.
func NewAdapter(config Config) (*Adapter, error) {
	calendarURL := config.CalendarURL
	if calendarURL == "" {
		calendarURL = CalendarURL
	}
	fetcher, err := sourcehttp.New(sourcehttp.Config{
		Resource:      resource,
		URL:           calendarURL,
		Accept:        "text/html",
		Client:        config.Client,
		RequestBudget: config.RequestBudget,
	})
	if err != nil {
		if errors.Is(err, sourcehttp.ErrNegativeRequestBudget) {
			return nil, errors.New("request budget must not be negative")
		}
		return nil, err
	}

	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{fetcher: fetcher, now: now}, nil
}

// FetchEvents retrieves the configured page and returns supported unique retail-sales releases.
func (adapter *Adapter) FetchEvents(ctx context.Context) ([]calendar.Event, error) {
	body, err := adapter.fetcher.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	events, err := parseEvents(body, adapter.now().UTC())
	if err != nil {
		return nil, fmt.Errorf("parse Census calendar: %w", err)
	}
	return events, nil
}
