// Package eurostat retrieves and normalizes supported Eurostat economic releases.
package eurostat

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehttp"
)

const (
	// Source is the normalized name of the Eurostat calendar source.
	Source = "eurostat"
	// CalendarURL is the canonical public Euro indicators release calendar.
	CalendarURL = "https://ec.europa.eu/eurostat/news/euro-indicators/release-calendar"
	// EventsURL is the official JSON endpoint backing the release calendar.
	EventsURL = "https://ec.europa.eu/eurostat/o/calendars/eventsJson"

	resource = "Eurostat calendar"
)

// HTTPClient is the subset of http.Client used to retrieve the calendar.
type HTTPClient = sourcehttp.Client

// Config controls calendar retrieval and provides deterministic test seams.
type Config struct {
	EventsURL     string
	Client        HTTPClient
	Now           func() time.Time
	RequestBudget time.Duration
}

// Adapter retrieves and normalizes the Eurostat release calendar.
type Adapter struct {
	fetcher  *sourcehttp.Fetcher
	now      func() time.Time
	location *time.Location
}

// NewAdapter validates config and returns a Eurostat calendar adapter.
func NewAdapter(config Config) (*Adapter, error) {
	eventsURL := config.EventsURL
	if eventsURL == "" {
		eventsURL = EventsURL
	}
	fetcher, err := sourcehttp.New(sourcehttp.Config{
		Resource:      resource,
		URL:           eventsURL,
		Accept:        "application/json",
		Client:        config.Client,
		RequestBudget: config.RequestBudget,
	})
	if err != nil {
		if errors.Is(err, sourcehttp.ErrNegativeRequestBudget) {
			return nil, errors.New("request budget must not be negative")
		}
		return nil, err
	}

	location, err := time.LoadLocation("Europe/Luxembourg")
	if err != nil {
		return nil, fmt.Errorf("load Eurostat calendar time zone: %w", err)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Adapter{fetcher: fetcher, now: now, location: location}, nil
}

// FetchEvents retrieves the current calendar year and returns supported unique releases.
func (adapter *Adapter) FetchEvents(ctx context.Context) ([]calendar.Event, error) {
	queryTime := adapter.now()
	body, err := adapter.fetcher.FetchWithQuery(ctx, adapter.calendarQuery(queryTime))
	if err != nil {
		return nil, err
	}

	events, err := parseEvents(body, adapter.now().UTC())
	if err != nil {
		return nil, fmt.Errorf("parse Eurostat calendar: %w", err)
	}
	return events, nil
}

func (adapter *Adapter) calendarQuery(now time.Time) url.Values {
	year := now.In(adapter.location).Year()
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, adapter.location)
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, adapter.location)
	return url.Values{
		"authorExclude":   {""},
		"authorInclude":   {""},
		"category":        {"0"},
		"end":             {end.Format(time.RFC3339)},
		"isEuroindicator": {strconv.FormatBool(true)},
		"keywords":        {""},
		"start":           {start.Format(time.RFC3339)},
		"theme":           {"0"},
	}
}
