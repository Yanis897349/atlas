// Package bea retrieves and normalizes scheduled Bureau of Economic Analysis GDP releases.
package bea

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehttp"
)

const (
	// Source is the normalized name of the Bureau of Economic Analysis calendar source.
	Source = "bea"
	// CalendarURL is the canonical Bureau of Economic Analysis full release schedule.
	CalendarURL = "https://www.bea.gov/news/schedule/full"

	resource = "BEA calendar"
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

// Adapter retrieves and normalizes the BEA release schedule.
type Adapter struct {
	fetcher *sourcehttp.Fetcher
	now     func() time.Time
}

// NewAdapter validates config and returns a BEA calendar adapter.
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

// FetchEvents retrieves the configured page and returns supported unique GDP releases.
func (adapter *Adapter) FetchEvents(ctx context.Context) ([]calendar.Event, error) {
	body, err := adapter.fetcher.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	events, err := parseEvents(body, adapter.now().UTC())
	if err != nil {
		return nil, fmt.Errorf("parse BEA calendar: %w", err)
	}
	return events, nil
}
