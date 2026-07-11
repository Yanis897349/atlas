package fed

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/calendar/sourcehtml"
	"golang.org/x/net/html"
)

const eventName = "Federal Open Market Committee Interest Rate Decision"

var (
	dateRangePattern = regexp.MustCompile(`^(\d{1,2})\s*-\s*(\d{1,2})\s*\*?$`)
	monthNumbers     = map[string]time.Month{
		"jan": time.January, "january": time.January,
		"feb": time.February, "february": time.February,
		"mar": time.March, "march": time.March,
		"apr": time.April, "april": time.April,
		"may": time.May,
		"jun": time.June, "june": time.June,
		"jul": time.July, "july": time.July,
		"aug": time.August, "august": time.August,
		"sep": time.September, "sept": time.September, "september": time.September,
		"oct": time.October, "october": time.October,
		"nov": time.November, "november": time.November,
		"dec": time.December, "december": time.December,
	}
)

func normalizeMeeting(
	row *html.Node,
	year int,
	eastern *time.Location,
	retrievedAt time.Time,
) (calendar.Event, bool, error) {
	dateNode := sourcehtml.FirstNodeWithClass(row, "fomc-meeting__date")
	if dateNode == nil {
		return calendar.Event{}, false, errors.New("meeting date is required")
	}
	dateText := sourcehtml.NormalizedText(dateNode)
	if strings.Contains(strings.ToLower(dateText), "notation vote") {
		return calendar.Event{}, false, nil
	}

	monthNode := sourcehtml.FirstNodeWithClass(row, "fomc-meeting__month")
	if monthNode == nil {
		return calendar.Event{}, false, errors.New("meeting month is required")
	}
	startMonth, endMonth, err := parseMonths(sourcehtml.NormalizedText(monthNode))
	if err != nil {
		return calendar.Event{}, false, err
	}
	startDay, endDay, err := parseDateRange(dateText)
	if err != nil {
		return calendar.Event{}, false, err
	}

	endYear := year
	if endMonth < startMonth {
		endYear++
	}
	startDate, err := validDate(year, startMonth, startDay)
	if err != nil {
		return calendar.Event{}, false, fmt.Errorf("invalid meeting start: %w", err)
	}
	endDate, err := validDate(endYear, endMonth, endDay)
	if err != nil {
		return calendar.Event{}, false, fmt.Errorf("invalid meeting end: %w", err)
	}
	if endDate.Before(startDate) {
		return calendar.Event{}, false, errors.New("meeting end precedes meeting start")
	}
	if endDate.Sub(startDate) > 7*24*time.Hour {
		return calendar.Event{}, false, errors.New("meeting range exceeds seven days")
	}

	scheduledAt := time.Date(endYear, endMonth, endDay, 14, 0, 0, 0, eastern).UTC()
	decisionDate := scheduledAt.In(eastern).Format(time.DateOnly)
	return calendar.Event{
		Source:          Source,
		ExternalEventID: "fomc-" + decisionDate,
		Name:            eventName,
		Region:          calendar.RegionUnitedStates,
		Type:            calendar.EventTypeInterestRateDecision,
		ScheduledAt:     scheduledAt,
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, true, nil
}

func parseMonths(value string) (time.Month, time.Month, error) {
	parts := strings.Split(value, "/")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, 0, fmt.Errorf("invalid meeting month %q", value)
	}
	startMonth, ok := monthNumbers[strings.ToLower(strings.TrimSpace(parts[0]))]
	if !ok {
		return 0, 0, fmt.Errorf("invalid meeting month %q", value)
	}
	if len(parts) == 1 {
		return startMonth, startMonth, nil
	}
	endMonth, ok := monthNumbers[strings.ToLower(strings.TrimSpace(parts[1]))]
	if !ok || endMonth != startMonth%12+1 {
		return 0, 0, fmt.Errorf("invalid cross-month meeting %q", value)
	}
	return startMonth, endMonth, nil
}

func parseDateRange(value string) (int, int, error) {
	matches := dateRangePattern.FindStringSubmatch(strings.TrimSpace(value))
	if matches == nil {
		return 0, 0, fmt.Errorf("invalid meeting date range %q", value)
	}
	startDay, startErr := strconv.Atoi(matches[1])
	endDay, endErr := strconv.Atoi(matches[2])
	if startErr != nil || endErr != nil {
		return 0, 0, fmt.Errorf("invalid meeting date range %q", value)
	}
	return startDay, endDay, nil
}

func validDate(year int, month time.Month, day int) (time.Time, error) {
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if date.Year() != year || date.Month() != month || date.Day() != day {
		return time.Time{}, fmt.Errorf("invalid date %04d-%02d-%02d", year, month, day)
	}
	return date, nil
}
