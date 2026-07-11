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

const eventIdentityPrefix = "eurostat-gdp-"

var periodPattern = regexp.MustCompile(`^Q([1-4])/(\d{4})$`)

type release struct {
	Period string `json:"period"`
	Start  string `json:"start"`
	Title  string `json:"title"`
}

type releaseIdentity struct {
	stage         string
	quarter       int
	referenceYear int
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
			return nil, fmt.Errorf("release %d title is required", index+1)
		}
		stage, supported := supportedStage(title)
		if !supported {
			continue
		}
		identity, err := normalizeIdentity(current.Period, stage)
		if err != nil {
			return nil, fmt.Errorf("normalize Eurostat release %d: %w", index+1, err)
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

func normalizeIdentity(period, stage string) (releaseIdentity, error) {
	trimmed := strings.TrimSpace(period)
	matches := periodPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return releaseIdentity{}, fmt.Errorf("invalid GDP reference period %q", trimmed)
	}
	referenceYear, err := strconv.Atoi(matches[2])
	if err != nil {
		return releaseIdentity{}, fmt.Errorf("invalid GDP reference period %q: %w", trimmed, err)
	}
	return releaseIdentity{
		stage:         stage,
		quarter:       int(matches[1][0] - '0'),
		referenceYear: referenceYear,
	}, nil
}

func normalizeRelease(current release, title string, identity releaseIdentity, retrievedAt time.Time) (calendar.Event, error) {
	start := strings.TrimSpace(current.Start)
	if start == "" {
		return calendar.Event{}, errors.New("release start is required")
	}
	scheduledAt, err := time.Parse(time.RFC3339Nano, start)
	if err != nil {
		return calendar.Event{}, fmt.Errorf("invalid release start %q: %w", start, err)
	}

	return calendar.Event{
		Source:          Source,
		ExternalEventID: fmt.Sprintf("%s%d-q%d-%s", eventIdentityPrefix, identity.referenceYear, identity.quarter, identity.stage),
		Name:            title,
		Region:          calendar.RegionEurozone,
		Type:            calendar.EventTypeGDP,
		ScheduledAt:     scheduledAt.UTC(),
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, nil
}
