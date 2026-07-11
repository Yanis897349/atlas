// Package sourcehtml provides deterministic traversal helpers for HTML calendar sources.
package sourcehtml

import (
	"strings"

	"golang.org/x/net/html"
)

// NodesWithClass returns matching elements in pre-order.
func NodesWithClass(root *html.Node, className string) []*html.Node {
	nodes := make([]*html.Node, 0)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.ElementNode && hasClass(node, className) {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

// FirstNodeWithClass returns the first matching element in pre-order.
func FirstNodeWithClass(root *html.Node, className string) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && hasClass(root, className) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := FirstNodeWithClass(child, className); match != nil {
			return match
		}
	}
	return nil
}

// FirstElementWithID returns the first element with the supplied ID in pre-order.
func FirstElementWithID(root *html.Node, id string) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && attribute(root, "id") == id {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := FirstElementWithID(child, id); match != nil {
			return match
		}
	}
	return nil
}

// FirstElement returns the first element with the supplied name in pre-order.
func FirstElement(root *html.Node, name string) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && root.Data == name {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := FirstElement(child, name); match != nil {
			return match
		}
	}
	return nil
}

// ChildElements returns direct child elements with the supplied name.
func ChildElements(root *html.Node, name string) []*html.Node {
	nodes := make([]*html.Node, 0)
	if root == nil {
		return nodes
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == name {
			nodes = append(nodes, child)
		}
	}
	return nodes
}

// NormalizedText returns descendant text with whitespace collapsed to single spaces.
func NormalizedText(node *html.Node) string {
	if node == nil {
		return ""
	}
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

func hasClass(node *html.Node, className string) bool {
	for value := range strings.FieldsSeq(attribute(node, "class")) {
		if value == className {
			return true
		}
	}
	return false
}

func attribute(node *html.Node, name string) string {
	for _, current := range node.Attr {
		if current.Key == name {
			return current.Val
		}
	}
	return ""
}
