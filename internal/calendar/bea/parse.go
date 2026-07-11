package bea

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/Yanis897349/atlas/internal/calendar"
	"golang.org/x/net/html"
)

const eventIdentityPrefix = "bea-gdp-"

var (
	yearHeadingPattern = regexp.MustCompile(`^Year (\d{4})$`)
	gdpTitlePattern    = regexp.MustCompile(`^GDP \((Advance|Second|Third) Estimate\).*\b(1st|2nd|3rd|4th) Quarter(?: and Year)? (\d{4})(?:[;,].*)?$`)
)

type releaseIdentity struct {
	estimate      string
	quarter       int
	referenceYear int
}

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	table := firstElementWithID(document, "release-schedule-table")
	if table == nil {
		return nil, errors.New("release schedule table is required")
	}
	year, err := releaseYear(table)
	if err != nil {
		return nil, err
	}
	tbody := firstElement(table, "tbody")
	if tbody == nil {
		return nil, errors.New("release schedule body is required")
	}
	rows := childElements(tbody, "tr")
	if len(rows) == 0 {
		return nil, errors.New("release schedule rows are required")
	}

	eastern, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("load release time zone: %w", err)
	}

	events := make([]calendar.Event, 0)
	seen := make(map[string]struct{})
	for index, row := range rows {
		titleNode := firstNodeWithClass(row, "release-title")
		if titleNode == nil {
			return nil, fmt.Errorf("release %d title is required", index+1)
		}
		title := normalizedText(titleNode)
		identity, supported, err := supportedGDPRelease(title)
		if err != nil {
			return nil, fmt.Errorf("normalize BEA release %d: %w", index+1, err)
		}
		if !supported {
			continue
		}

		event, err := normalizeRelease(row, year, title, identity, eastern, retrievedAt)
		if err != nil {
			return nil, fmt.Errorf("normalize BEA release %d: %w", index+1, err)
		}
		if _, exists := seen[event.ExternalEventID]; exists {
			continue
		}
		seen[event.ExternalEventID] = struct{}{}
		events = append(events, event)
	}
	return events, nil
}

func releaseYear(table *html.Node) (int, error) {
	thead := firstElement(table, "thead")
	if thead == nil {
		return 0, errors.New("release schedule heading is required")
	}
	heading := firstElement(thead, "th")
	if heading == nil {
		return 0, errors.New("release schedule year heading is required")
	}
	value := normalizedText(heading)
	matches := yearHeadingPattern.FindStringSubmatch(value)
	if matches == nil {
		return 0, fmt.Errorf("invalid release schedule year heading %q", value)
	}
	year, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid release schedule year heading %q: %w", value, err)
	}
	return year, nil
}

func supportedGDPRelease(title string) (releaseIdentity, bool, error) {
	if !strings.HasPrefix(title, "GDP (Advance Estimate)") &&
		!strings.HasPrefix(title, "GDP (Second Estimate)") &&
		!strings.HasPrefix(title, "GDP (Third Estimate)") {
		return releaseIdentity{}, false, nil
	}

	matches := gdpTitlePattern.FindStringSubmatch(title)
	if matches == nil {
		return releaseIdentity{}, false, fmt.Errorf("invalid national GDP release title %q", title)
	}
	quarter := int(matches[2][0] - '0')
	referenceYear, err := strconv.Atoi(matches[3])
	if err != nil {
		return releaseIdentity{}, false, fmt.Errorf("invalid national GDP release title %q: %w", title, err)
	}
	return releaseIdentity{
		estimate:      strings.ToLower(matches[1]),
		quarter:       quarter,
		referenceYear: referenceYear,
	}, true, nil
}

func normalizeRelease(
	row *html.Node,
	releaseYear int,
	title string,
	identity releaseIdentity,
	eastern *time.Location,
	retrievedAt time.Time,
) (calendar.Event, error) {
	dateNode := firstNodeWithClass(row, "release-date")
	if dateNode == nil {
		return calendar.Event{}, errors.New("release date is required")
	}
	dateText := normalizedText(dateNode)

	timeNode := firstElement(firstNodeWithClass(row, "scheduled-date"), "small")
	if timeNode == nil {
		return calendar.Event{}, errors.New("release time is required")
	}
	timeText := normalizedText(timeNode)
	scheduledAt, err := time.ParseInLocation("January 2 2006 3:04 PM", fmt.Sprintf("%s %d %s", dateText, releaseYear, timeText), eastern)
	if err != nil {
		return calendar.Event{}, fmt.Errorf("invalid release date and time %q: %w", dateText+" "+timeText, err)
	}

	externalEventID := fmt.Sprintf(
		"%s%d-q%d-%s",
		eventIdentityPrefix,
		identity.referenceYear,
		identity.quarter,
		identity.estimate,
	)
	return calendar.Event{
		Source:          Source,
		ExternalEventID: externalEventID,
		Name:            title,
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeGDP,
		ScheduledAt:     scheduledAt.UTC(),
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, nil
}
