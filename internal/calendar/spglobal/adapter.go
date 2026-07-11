// Package spglobal retrieves and normalizes scheduled S&P Global PMI releases.
package spglobal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehttp"
)

const (
	// Source is the normalized name of the S&P Global PMI calendar source.
	Source = "sp_global"
	// CalendarURL is the canonical public S&P Global PMI release calendar.
	CalendarURL = "https://www.pmi.spglobal.com/Public/Release/ReleaseDates?language=en"

	resource = "S&P Global PMI calendar"
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

// Adapter retrieves and normalizes the S&P Global PMI release calendar.
type Adapter struct {
	fetcher *sourcehttp.Fetcher
	now     func() time.Time
}

// NewAdapter validates config and returns an S&P Global PMI calendar adapter.
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

// FetchEvents retrieves the configured page and returns supported unique flash releases.
func (adapter *Adapter) FetchEvents(ctx context.Context) ([]calendar.Event, error) {
	body, err := adapter.fetcher.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	events, err := parseEvents(body, adapter.now().UTC())
	if err != nil {
		return nil, fmt.Errorf("parse S&P Global PMI calendar: %w", err)
	}
	return events, nil
}
