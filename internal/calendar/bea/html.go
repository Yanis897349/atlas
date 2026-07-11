package bea

import (
	"strings"

	"golang.org/x/net/html"
)

func firstElementWithID(root *html.Node, id string) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && attribute(root, "id") == id {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := firstElementWithID(child, id); match != nil {
			return match
		}
	}
	return nil
}

func firstNodeWithClass(root *html.Node, className string) *html.Node {
	if root == nil {
		return nil
	}
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
	if root == nil {
		return nil
	}
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

func childElements(root *html.Node, name string) []*html.Node {
	nodes := make([]*html.Node, 0)
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == name {
			nodes = append(nodes, child)
		}
	}
	return nodes
}

func attribute(node *html.Node, name string) string {
	for _, current := range node.Attr {
		if current.Key == name {
			return current.Val
		}
	}
	return ""
}

func hasClass(node *html.Node, className string) bool {
	for value := range strings.FieldsSeq(attribute(node, "class")) {
		if value == className {
			return true
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
