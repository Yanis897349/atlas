package spglobal

import (
	"bytes"
	"errors"
	"strings"

	"github.com/Yanis897349/atlas/internal/calendar/sourcehtml"
	"golang.org/x/net/html"
)

const (
	calendarHeading = "Calendar"
	upcomingHeading = "Upcoming"
)

func parseCalendarDocument(body []byte) ([]string, error) {
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
	return calendarWords(root, upcoming), nil
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

func isHeading(name string) bool {
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

func excludedCalendarElement(name string) bool {
	switch name {
	case "footer", "header", "nav", "script", "style":
		return true
	default:
		return false
	}
}
