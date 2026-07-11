package fed

import (
	"strings"

	"golang.org/x/net/html"
)

func nodesWithClass(root *html.Node, className string) []*html.Node {
	nodes := make([]*html.Node, 0)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
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
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
			text.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(text.String()), " ")
}
