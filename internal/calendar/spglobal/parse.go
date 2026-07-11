package spglobal

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

const eventName = "S&P Global Flash Eurozone PMI"

var (
	yearPattern = regexp.MustCompile(`^\d{4}$`)
	wordPattern = regexp.MustCompile(`^[A-Za-z]+$`)
	dayPattern  = regexp.MustCompile(`^\d{1,2}$`)
	eventWords  = strings.Fields(eventName)
)

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	words, err := parseCalendarDocument(body)
	if err != nil {
		return nil, err
	}
	return parseCalendarWords(words, retrievedAt)
}

func parseCalendarWords(words []string, retrievedAt time.Time) ([]calendar.Event, error) {
	events := make([]calendar.Event, 0)
	seen := make(map[string]struct{})
	year := 0
	dateText := ""
	timeText := ""
	hasDate := false
	hasTime := false
	supportedIndex := 0
	for index := 0; index < len(words); index++ {
		switch {
		case yearPattern.MatchString(words[index]):
			year, _ = strconv.Atoi(words[index])
			dateText = ""
			timeText = ""
		case index+1 < len(words) && wordPattern.MatchString(words[index]) && dayPattern.MatchString(words[index+1]):
			dateText = words[index] + " " + words[index+1]
			hasDate = true
			timeText = ""
			index++
		case index+1 < len(words) && words[index+1] == "UTC":
			timeText = words[index]
			hasTime = true
			index++
		case matchesWords(words[index:], eventWords):
			supportedIndex++
			event, err := normalizeRelease(year, dateText, timeText, retrievedAt)
			if err != nil {
				return nil, fmt.Errorf("normalize S&P Global release %d: %w", supportedIndex, err)
			}
			if _, exists := seen[event.ExternalEventID]; !exists {
				seen[event.ExternalEventID] = struct{}{}
				events = append(events, event)
			}
			index += len(eventWords) - 1
		}
	}
	if year == 0 {
		return nil, errors.New("calendar year is required")
	}
	if !hasDate {
		return nil, errors.New("calendar date is required")
	}
	if !hasTime {
		return nil, errors.New("calendar releases are required")
	}
	return events, nil
}

func matchesWords(words, wanted []string) bool {
	if len(words) < len(wanted) {
		return false
	}
	for index := range wanted {
		if words[index] != wanted[index] {
			return false
		}
	}
	return true
}

func normalizeRelease(year int, dateText, timeText string, retrievedAt time.Time) (calendar.Event, error) {
	if year == 0 {
		return calendar.Event{}, errors.New("release year is required")
	}
	if dateText == "" {
		return calendar.Event{}, errors.New("release date is required")
	}
	if timeText == "" {
		return calendar.Event{}, errors.New("release time is required")
	}
	scheduledAt, err := time.Parse("2006 January 2 15:04", fmt.Sprintf("%d %s %s", year, dateText, timeText))
	if err != nil {
		return calendar.Event{}, fmt.Errorf("invalid UTC release date and time %q: %w", fmt.Sprintf("%d %s %s", year, dateText, timeText), err)
	}

	return calendar.Event{
		Source:          Source,
		ExternalEventID: fmt.Sprintf("eurozone-flash-pmi-%04d-%02d", scheduledAt.Year(), scheduledAt.Month()),
		Name:            eventName,
		Region:          calendar.RegionEurozone,
		Type:            calendar.EventTypePMI,
		ScheduledAt:     scheduledAt.UTC(),
		SourceURL:       CalendarURL,
		RetrievedAt:     retrievedAt,
	}, nil
}
