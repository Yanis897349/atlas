package sourcehtml

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestTraversalFindsElementsDeterministically(t *testing.T) {
	document := parseDocument(t, `
<main>
  <section id="schedule" class="panel featured">
    <h2 class="heading primary"> First <span>release</span> </h2>
    <div><article class="entry target">Nested entry</article></div>
    <article class="entry">Direct entry</article>
  </section>
  <section class="panel"><article class="entry target">Later entry</article></section>
</main>`)

	panels := NodesWithClass(document, "panel")
	if len(panels) != 2 || attribute(panels[0], "id") != "schedule" {
		t.Fatalf("NodesWithClass(panel) returned %d nodes in unexpected order", len(panels))
	}
	if got := FirstNodeWithClass(document, "target"); NormalizedText(got) != "Nested entry" {
		t.Errorf("FirstNodeWithClass(target) text = %q, want Nested entry", NormalizedText(got))
	}
	schedule := FirstElementWithID(document, "schedule")
	if schedule == nil || schedule.Data != "section" {
		t.Fatalf("FirstElementWithID(schedule) = %#v, want section", schedule)
	}
	if got := FirstElement(schedule, "span"); NormalizedText(got) != "release" {
		t.Errorf("FirstElement(span) text = %q, want release", NormalizedText(got))
	}
	children := ChildElements(schedule, "article")
	if len(children) != 1 || NormalizedText(children[0]) != "Direct entry" {
		t.Errorf("ChildElements(article) = %#v, want only direct article", children)
	}
	if got := NormalizedText(FirstNodeWithClass(document, "heading")); got != "First release" {
		t.Errorf("NormalizedText(heading) = %q, want First release", got)
	}
}

func TestTraversalHandlesMissingRootsAndMatchesWholeClassTokens(t *testing.T) {
	document := parseDocument(t, `<div class="calendar-panel"></div>`)
	if FirstNodeWithClass(document, "panel") != nil {
		t.Error("FirstNodeWithClass(panel) matched a partial class token")
	}
	if FirstNodeWithClass(nil, "panel") != nil || FirstElementWithID(nil, "schedule") != nil || FirstElement(nil, "div") != nil {
		t.Error("first-match traversal returned a node for a nil root")
	}
	if nodes := NodesWithClass(nil, "panel"); len(nodes) != 0 {
		t.Errorf("NodesWithClass(nil) returned %d nodes, want none", len(nodes))
	}
	if children := ChildElements(nil, "div"); len(children) != 0 {
		t.Errorf("ChildElements(nil) returned %d nodes, want none", len(children))
	}
	if got := NormalizedText(nil); got != "" {
		t.Errorf("NormalizedText(nil) = %q, want empty", got)
	}
}

func parseDocument(t *testing.T, source string) *html.Node {
	t.Helper()
	document, err := html.Parse(strings.NewReader(source))
	if err != nil {
		t.Fatalf("html.Parse() error = %v", err)
	}
	return document
}
