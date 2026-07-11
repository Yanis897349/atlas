package ecb

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/Yanis897349/atlas/internal/calendar"
	"golang.org/x/net/html"
)

const (
	eventName                 = "European Central Bank Interest Rate Decision"
	decisionDescription       = "Governing Council of the ECB: monetary policy meeting"
	decisionDescriptionSuffix = "(Day 2), followed by press conference"
)

var calendarDatePattern = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`)

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	schedule := firstNodeWithClass(document, "definition-list")
	if schedule == nil {
		return nil, errors.New("meeting schedule is required")
	}
	list := firstElement(schedule, "dl")
	if list == nil {
		return nil, errors.New("meeting schedule list is required")
	}

	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return nil, fmt.Errorf("load decision time zone: %w", err)
	}

	events := make([]calendar.Event, 0)
	seen := make(map[string]struct{})
	pendingDate := ""
	rows := 0
	for node := list.FirstChild; node != nil; node = node.NextSibling {
		if node.Type != html.ElementNode {
			continue
		}
		switch node.Data {
		case "dt":
			if pendingDate != "" {
				return nil, errors.New("meeting date is missing a description")
			}
			pendingDate = normalizedText(node)
		case "dd":
			if pendingDate == "" {
				return nil, errors.New("meeting description is missing a date")
			}
			rows++
			description := normalizedText(node)
			if supportedDescription(description) {
				event, err := normalizeDecision(pendingDate, berlin, retrievedAt)
				if err != nil {
					return nil, fmt.Errorf("normalize ECB meeting %d: %w", rows, err)
				}
				if _, exists := seen[event.ExternalEventID]; !exists {
					seen[event.ExternalEventID] = struct{}{}
					events = append(events, event)
				}
			}
			pendingDate = ""
		}
	}
	if pendingDate != "" {
		return nil, errors.New("meeting date is missing a description")
	}
	if rows == 0 {
		return nil, errors.New("meeting schedule entries are required")
	}
	return events, nil
}

func supportedDescription(description string) bool {
	return strings.HasPrefix(description, decisionDescription) &&
		strings.HasSuffix(description, decisionDescriptionSuffix)
}

func normalizeDecision(dateText string, berlin *time.Location, retrievedAt time.Time) (calendar.Event, error) {
	dateText = strings.TrimSpace(dateText)
	if !calendarDatePattern.MatchString(dateText) {
		return calendar.Event{}, fmt.Errorf("invalid meeting date %q", dateText)
	}
	date, err := time.Parse("02/01/2006", dateText)
	if err != nil {
		return calendar.Event{}, fmt.Errorf("invalid meeting date %q: %w", dateText, err)
	}

	scheduledAt := time.Date(date.Year(), date.Month(), date.Day(), 14, 15, 0, 0, berlin).UTC()
	decisionDate := date.Format(time.DateOnly)
	return calendar.Event{
		Source:          Source,
		ExternalEventID: "ecb-" + decisionDate,
		Name:            eventName,
		Region:          calendar.RegionEurozone,
		Type:            calendar.EventTypeInterestRateDecision,
		ScheduledAt:     scheduledAt,
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, nil
}
