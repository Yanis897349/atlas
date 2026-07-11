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

const (
	eventName       = "S&P Global Flash Eurozone PMI"
	calendarHeading = "Calendar"
	upcomingHeading = "Upcoming"
)

var (
	yearPattern = regexp.MustCompile(`^\d{4}$`)
	wordPattern = regexp.MustCompile(`^[A-Za-z]+$`)
	dayPattern  = regexp.MustCompile(`^\d{1,2}$`)
	eventWords  = strings.Fields(eventName)
)

func parseEvents(body []byte, retrievedAt time.Time) ([]calendar.Event, error) {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	heading := findCalendarHeading(document)
	if heading == nil {
		return nil, errors.New("calendar heading is required")
	}
	upcoming := findUpcomingMarker(document, heading)
	if upcoming == nil {
		return nil, errors.New("upcoming calendar is required")
	}
	root := commonAncestor(heading, upcoming)
	if root == nil {
		return nil, errors.New("calendar structure is required")
	}
	return parseCalendarWords(calendarWords(root, upcoming), retrievedAt)
}

func findUpcomingMarker(root, heading *html.Node) *html.Node {
	started := false
	var marker *html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || marker != nil {
			return
		}
		if node == heading {
			started = true
			return
		}
		if started && node.Type == html.TextNode && strings.Join(strings.Fields(node.Data), " ") == upcomingHeading {
			marker = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return marker
}

func commonAncestor(first, second *html.Node) *html.Node {
	ancestors := make(map[*html.Node]struct{})
	for node := first; node != nil; node = node.Parent {
		ancestors[node] = struct{}{}
	}
	for node := second; node != nil; node = node.Parent {
		if _, exists := ancestors[node]; exists {
			return node
		}
	}
	return nil
}

func findCalendarHeading(root *html.Node) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && isHeading(root.Data) && sourcehtml.NormalizedText(root) == calendarHeading {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if heading := findCalendarHeading(child); heading != nil {
			return heading
		}
	}
	return nil
}

func isHeading(name string) bool {
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

func calendarWords(root, marker *html.Node) []string {
	words := make([]string, 0)
	started := false
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node == marker {
			started = true
			return
		}
		if started && node.Type == html.ElementNode && excludedCalendarElement(node.Data) {
			return
		}
		if started && node.Type == html.TextNode {
			words = append(words, strings.Fields(node.Data)...)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return words
}

func excludedCalendarElement(name string) bool {
	switch name {
	case "footer", "header", "nav", "script", "style":
		return true
	default:
		return false
	}
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
