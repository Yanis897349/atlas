package fed

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
	"github.com/Yanis897349/atlas/internal/calendar/sourcehtml"
	"golang.org/x/net/html"
)

var yearHeadingPattern = regexp.MustCompile(`^(\d{4}) FOMC Meetings$`)

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	eastern, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("load decision time zone: %w", err)
	}

	panels := sourcehtml.NodesWithClass(document, "panel")
	events := make([]calendar.Event, 0)
	seen := make(map[string]struct{})
	calendarPanels := 0
	for _, panel := range panels {
		headingNode := sourcehtml.FirstNodeWithClass(panel, "panel-heading")
		if headingNode == nil {
			continue
		}
		heading := sourcehtml.NormalizedText(headingNode)
		if !strings.HasSuffix(heading, "FOMC Meetings") {
			continue
		}

		matches := yearHeadingPattern.FindStringSubmatch(heading)
		if matches == nil {
			return nil, fmt.Errorf("invalid FOMC year heading %q", heading)
		}
		year, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("invalid FOMC year heading %q: %w", heading, err)
		}

		calendarPanels++
		rows := sourcehtml.NodesWithClass(panel, "fomc-meeting")
		if len(rows) == 0 {
			return nil, fmt.Errorf("FOMC year %d has no meeting rows", year)
		}
		for rowIndex, row := range rows {
			event, supported, err := normalizeMeeting(row, year, eastern, retrievedAt)
			if err != nil {
				return nil, fmt.Errorf("normalize FOMC meeting %d for %d: %w", rowIndex+1, year, err)
			}
			if !supported {
				continue
			}
			if _, exists := seen[event.ExternalEventID]; exists {
				continue
			}
			seen[event.ExternalEventID] = struct{}{}
			events = append(events, event)
		}
	}

	if calendarPanels == 0 {
		return nil, errors.New("FOMC meeting panels are required")
	}
	return events, nil
}
