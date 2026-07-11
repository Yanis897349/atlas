package spglobal

import (
	"bytes"
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

const eventName = "S&P Global Flash Eurozone PMI"

var (
	yearPattern    = regexp.MustCompile(`^\d{4}$`)
	datePattern    = regexp.MustCompile(`^[A-Za-z]+\s+\d{1,2}$`)
	releasePattern = regexp.MustCompile(`^(\S+)\s+UTC\s+(.+)$`)
)

type calendarToken struct {
	kind tokenKind
	text string
	time string
	name string
}

type tokenKind uint8

const (
	tokenYear tokenKind = iota + 1
	tokenDate
	tokenRelease
)

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	tokens := calendarTokens(document)
	if err := validateCalendarTokens(tokens); err != nil {
		return nil, err
	}

	events := make([]calendar.Event, 0)
	seen := make(map[string]struct{})
	year := 0
	dateText := ""
	releaseIndex := 0
	for _, token := range tokens {
		switch token.kind {
		case tokenYear:
			year, _ = strconv.Atoi(token.text)
			dateText = ""
		case tokenDate:
			dateText = token.text
		case tokenRelease:
			releaseIndex++
			if token.name != eventName {
				continue
			}
			event, err := normalizeRelease(year, dateText, token.time, retrievedAt)
			if err != nil {
				return nil, fmt.Errorf("normalize S&P Global release %d: %w", releaseIndex, err)
			}
			if _, exists := seen[event.ExternalEventID]; exists {
				continue
			}
			seen[event.ExternalEventID] = struct{}{}
			events = append(events, event)
		}
	}
	return events, nil
}

func calendarTokens(document *html.Node) []calendarToken {
	tokens := make([]calendarToken, 0)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.ElementNode {
			text := sourcehtml.NormalizedText(node)
			if token, ok := classifyToken(text); ok {
				tokens = append(tokens, token)
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return tokens
}

func classifyToken(text string) (calendarToken, bool) {
	switch {
	case yearPattern.MatchString(text):
		return calendarToken{kind: tokenYear, text: text}, true
	case datePattern.MatchString(text):
		return calendarToken{kind: tokenDate, text: text}, true
	case text == eventName:
		return calendarToken{kind: tokenRelease, name: text}, true
	case strings.Count(text, " UTC ") == 1:
		matches := releasePattern.FindStringSubmatch(text)
		if matches != nil {
			return calendarToken{kind: tokenRelease, time: matches[1], name: strings.TrimSpace(matches[2])}, true
		}
	}
	return calendarToken{}, false
}

func validateCalendarTokens(tokens []calendarToken) error {
	hasYear := false
	hasDate := false
	hasRelease := false
	for _, token := range tokens {
		switch token.kind {
		case tokenYear:
			hasYear = true
		case tokenDate:
			hasDate = true
		case tokenRelease:
			hasRelease = true
		}
	}
	if !hasYear {
		return errors.New("calendar year is required")
	}
	if !hasDate {
		return errors.New("calendar date is required")
	}
	if !hasRelease {
		return errors.New("calendar releases are required")
	}
	return nil
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
