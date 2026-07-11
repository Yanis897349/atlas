package eurostat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

const (
	gdpIdentityPrefix         = "eurostat-gdp-"
	retailSalesIdentityPrefix = "eurostat-retail-sales-"
	retailSalesTitle          = "Retail trade"
)

var periodPattern = regexp.MustCompile(`^Q([1-4])/(\d{4})$`)

const releaseTimeWithoutSeconds = "2006-01-02T15:04Z07:00"

type release struct {
	Period string `json:"period"`
	Start  string `json:"start"`
	Title  string `json:"title"`
}

type eventIdentity struct {
	externalEventID string
	eventType       calendar.EventType
}

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, errors.New("release list is required")
	}
	var releases []release
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, err
	}

	events := make([]calendar.Event, 0)
	seen := make(map[string]struct{})
	for index, current := range releases {
		title := strings.TrimSpace(current.Title)
		if title == "" {
			continue
		}
		identity, supported, err := normalizeIdentity(title, current.Period)
		if err != nil {
			return nil, fmt.Errorf("normalize Eurostat release %d: %w", index+1, err)
		}
		if !supported {
			continue
		}
		event, err := normalizeRelease(current, title, identity, retrievedAt)
		if err != nil {
			return nil, fmt.Errorf("normalize Eurostat release %d: %w", index+1, err)
		}
		if _, exists := seen[event.ExternalEventID]; exists {
			continue
		}
		seen[event.ExternalEventID] = struct{}{}
		events = append(events, event)
	}
	return events, nil
}

func supportedStage(title string) (string, bool) {
	switch title {
	case "Preliminary flash estimate GDP - EU and euro area":
		return "preliminary-flash", true
	case "Flash estimate GDP and employment - EU and euro area":
		return "flash", true
	case "GDP main aggregates and employment":
		return "main-aggregates", true
	case "GDP main aggregates and employment - update":
		return "main-aggregates-update", true
	default:
		return "", false
	}
}

func normalizeIdentity(title, period string) (eventIdentity, bool, error) {
	if title == retailSalesTitle {
		identity, err := normalizeRetailSalesIdentity(period)
		return identity, true, err
	}
	stage, supported := supportedStage(title)
	if !supported {
		return eventIdentity{}, false, nil
	}
	identity, err := normalizeGDPIdentity(period, stage)
	return identity, true, err
}

func normalizeGDPIdentity(period, stage string) (eventIdentity, error) {
	trimmed := strings.TrimSpace(period)
	matches := periodPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return eventIdentity{}, fmt.Errorf("invalid GDP reference period %q", trimmed)
	}
	referenceYear, err := strconv.Atoi(matches[2])
	if err != nil {
		return eventIdentity{}, fmt.Errorf("invalid GDP reference period %q: %w", trimmed, err)
	}
	return eventIdentity{
		externalEventID: fmt.Sprintf("%s%d-q%c-%s", gdpIdentityPrefix, referenceYear, matches[1][0], stage),
		eventType:       calendar.EventTypeGDP,
	}, nil
}

func normalizeRetailSalesIdentity(period string) (eventIdentity, error) {
	trimmed := strings.TrimSpace(period)
	referenceMonth, err := time.Parse("January 2006", trimmed)
	if err != nil {
		return eventIdentity{}, fmt.Errorf("invalid retail sales reference period %q: %w", trimmed, err)
	}
	return eventIdentity{
		externalEventID: fmt.Sprintf("%s%04d-%02d", retailSalesIdentityPrefix, referenceMonth.Year(), referenceMonth.Month()),
		eventType:       calendar.EventTypeRetailSales,
	}, nil
}

func normalizeRelease(current release, title string, identity eventIdentity, retrievedAt time.Time) (calendar.Event, error) {
	start := strings.TrimSpace(current.Start)
	if start == "" {
		return calendar.Event{}, errors.New("release start is required")
	}
	scheduledAt, err := parseReleaseTime(start)
	if err != nil {
		return calendar.Event{}, fmt.Errorf("invalid release start %q: %w", start, err)
	}

	return calendar.Event{
		Source:          Source,
		ExternalEventID: identity.externalEventID,
		Name:            title,
		Region:          calendar.RegionEurozone,
		Type:            identity.eventType,
		ScheduledAt:     scheduledAt.UTC(),
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, nil
}

func parseReleaseTime(value string) (time.Time, error) {
	scheduledAt, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return scheduledAt, nil
	}
	return time.Parse(releaseTimeWithoutSeconds, value)
}
