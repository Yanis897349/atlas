// Package bls retrieves and normalizes scheduled Bureau of Labor Statistics releases.
package bls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	ics "github.com/arran4/golang-ical"
)

const (
	// Source is the normalized name of the Bureau of Labor Statistics calendar source.
	Source = "bls"
	// CalendarURL is the canonical Bureau of Labor Statistics release calendar feed.
	CalendarURL = "https://www.bls.gov/schedule/news_release/bls.ics"

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

// Adapter retrieves and normalizes the BLS release calendar.
type Adapter struct {
	calendarURL   string
	client        HTTPClient
	now           func() time.Time
	requestBudget time.Duration
}

// NewAdapter validates config and returns a BLS calendar adapter.
func NewAdapter(config Config) (*Adapter, error) {
	calendarURL := config.CalendarURL
	if calendarURL == "" {
		calendarURL = CalendarURL
	}
	validatedURL, err := validateHTTPURL(calendarURL)
	if err != nil {
		return nil, fmt.Errorf("invalid BLS calendar URL: %w", err)
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
		return nil, errors.New("BLS request budget must not be negative")
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

// FetchEvents retrieves the configured feed and returns supported unique releases.
func (adapter *Adapter) FetchEvents(ctx context.Context) ([]calendar.Event, error) {
	body, err := adapter.fetchBody(ctx)
	if err != nil {
		return nil, err
	}
	if !bytes.HasSuffix(bytes.TrimSpace(body), []byte("END:VCALENDAR")) {
		return nil, errors.New("parse BLS calendar: missing END:VCALENDAR")
	}

	document, err := ics.ParseCalendar(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse BLS calendar: %w", err)
	}

	retrievedAt := adapter.now().UTC()
	events := make([]calendar.Event, 0, len(document.Events()))
	seen := make(map[string]struct{}, len(document.Events()))
	for index, sourceEvent := range document.Events() {
		eventType, supported := supportedEventType(eventSummary(sourceEvent))
		if !supported {
			continue
		}

		event, err := normalizeEvent(sourceEvent, eventType, retrievedAt)
		if err != nil {
			return nil, fmt.Errorf("normalize BLS event %d: %w", index+1, err)
		}
		if _, exists := seen[event.ExternalEventID]; exists {
			continue
		}
		seen[event.ExternalEventID] = struct{}{}
		events = append(events, event)
	}

	return events, nil
}

func normalizeEvent(sourceEvent *ics.VEvent, eventType calendar.EventType, retrievedAt time.Time) (calendar.Event, error) {
	externalEventID := strings.TrimSpace(sourceEvent.Id())
	if externalEventID == "" {
		return calendar.Event{}, errors.New("UID is required")
	}
	scheduledAt, err := sourceEvent.GetStartAt()
	if err != nil || scheduledAt.IsZero() {
		return calendar.Event{}, errors.New("valid DTSTART is required")
	}

	return calendar.Event{
		Source:          Source,
		ExternalEventID: externalEventID,
		Name:            eventSummary(sourceEvent),
		Region:          calendar.RegionUnitedStates,
		Type:            eventType,
		ScheduledAt:     scheduledAt.UTC(),
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, nil
}

func eventSummary(event *ics.VEvent) string {
	property := event.GetProperty(ics.ComponentPropertySummary)
	if property == nil {
		return ""
	}
	return strings.TrimSpace(ics.FromText(property.Value))
}

func supportedEventType(summary string) (calendar.EventType, bool) {
	switch summary {
	case "Consumer Price Index":
		return calendar.EventTypeInflation, true
	case "Employment Situation":
		return calendar.EventTypeEmployment, true
	default:
		return "", false
	}
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
