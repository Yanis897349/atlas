package ecb

import (
	"strings"

	"golang.org/x/net/html"
)

func firstNodeWithClass(root *html.Node, className string) *html.Node {
	if root.Type == html.ElementNode && hasClass(root, className) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := firstNodeWithClass(child, className); match != nil {
			return match
		}
	}
	return nil
}

func firstElement(root *html.Node, name string) *html.Node {
	if root.Type == html.ElementNode && root.Data == name {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := firstElement(child, name); match != nil {
			return match
		}
	}
	return nil
}

func hasClass(node *html.Node, className string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key != "class" {
			continue
		}
		for value := range strings.FieldsSeq(attribute.Val) {
			if value == className {
				return true
			}
		}
	}
	return false
}

func normalizedText(node *html.Node) string {
	var value strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			value.WriteString(current.Data)
			value.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(value.String()), " ")
}
