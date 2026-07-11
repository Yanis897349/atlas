package census

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehtml"
	"golang.org/x/net/html"
)

const eventName = "Advance Monthly Sales for Retail and Food Services"

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	table := sourcehtml.FirstElementWithID(document, "calendar")
	if table == nil || table.Data != "table" {
		return nil, errors.New("release calendar table is required")
	}
	tableBody := sourcehtml.FirstElement(table, "tbody")
	if tableBody == nil {
		return nil, errors.New("release calendar body is required")
	}
	rows := sourcehtml.ChildElements(tableBody, "tr")
	if len(rows) == 0 {
		return nil, errors.New("release calendar rows are required")
	}

	eastern, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("load release time zone: %w", err)
	}

	events := make([]calendar.Event, 0)
	seen := make(map[string]struct{})
	releaseIndex := 0
	for _, row := range rows {
		if len(sourcehtml.ChildElements(row, "th")) > 0 {
			continue
		}
		cells := sourcehtml.ChildElements(row, "td")
		releaseIndex++
		if len(cells) == 0 {
			return nil, fmt.Errorf("release %d indicator is required", releaseIndex)
		}

		name := sourcehtml.NormalizedText(cells[0])
		if name == "" {
			return nil, fmt.Errorf("release %d indicator is required", releaseIndex)
		}
		if name != eventName {
			continue
		}
		if len(cells) < 2 {
			return nil, fmt.Errorf("normalize Census release %d: release date is required", releaseIndex)
		}
		dateText := sourcehtml.NormalizedText(cells[1])
		if dateText == "Suspended" {
			continue
		}
		if len(cells) < 4 {
			return nil, fmt.Errorf("normalize Census release %d: release date, time, and covered period are required", releaseIndex)
		}

		event, err := normalizeRelease(dateText, sourcehtml.NormalizedText(cells[2]), sourcehtml.NormalizedText(cells[3]), eastern, retrievedAt)
		if err != nil {
			return nil, fmt.Errorf("normalize Census release %d: %w", releaseIndex, err)
		}
		if _, exists := seen[event.ExternalEventID]; exists {
			continue
		}
		seen[event.ExternalEventID] = struct{}{}
		events = append(events, event)
	}
	return events, nil
}

func normalizeRelease(dateText, timeText, periodText string, eastern *time.Location, retrievedAt time.Time) (calendar.Event, error) {
	dateText = strings.TrimSpace(dateText)
	timeText = strings.TrimSpace(timeText)
	if dateText == "" {
		return calendar.Event{}, errors.New("release date is required")
	}
	if timeText == "" {
		return calendar.Event{}, errors.New("release time is required")
	}
	scheduledAt, err := time.ParseInLocation("January 2, 2006 3:04 PM", dateText+" "+timeText, eastern)
	if err != nil {
		return calendar.Event{}, fmt.Errorf("invalid release date and time %q: %w", dateText+" "+timeText, err)
	}

	periodText = strings.TrimSpace(periodText)
	period, err := time.Parse("January 2006", periodText)
	if err != nil {
		return calendar.Event{}, fmt.Errorf("invalid covered period %q: %w", periodText, err)
	}

	return calendar.Event{
		Source:          Source,
		ExternalEventID: fmt.Sprintf("retail-sales-%04d-%02d", period.Year(), period.Month()),
		Name:            eventName,
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeRetailSales,
		ScheduledAt:     scheduledAt.UTC(),
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, nil
}
