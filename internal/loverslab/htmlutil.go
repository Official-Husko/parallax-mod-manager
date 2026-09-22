package loverslab

import (
	"strings"

	"golang.org/x/net/html"
)

func isElement(n *html.Node, tag string) bool {
	return n.Type == html.ElementNode && n.Data == tag
}

func attrOr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attrOr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// find returns every node in the tree rooted at n for which match returns true.
func find(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if match(n) {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

// findOne returns the first node in the tree rooted at n for which match
// returns true, or nil.
func findOne(n *html.Node, match func(*html.Node) bool) *html.Node {
	var result *html.Node
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if match(n) {
			result = n
			return true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(n)
	return result
}

// text concatenates all text nodes under n.
func text(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// richText renders an IPS "rich text" fragment (the format used for
// changelog bodies, file descriptions, etc: paragraphs, <br> line breaks,
// and links) as readable plain text, instead of the run-together string
// text() would produce.
func richText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			sb.WriteString(n.Data)
			return
		case isElement(n, "br"):
			sb.WriteString("\n")
			return
		case isElement(n, "a"):
			href := attrOr(n, "href")
			label := strings.TrimSpace(text(n))
			sb.WriteString(label)
			if href != "" && href != label {
				sb.WriteString(" (" + href + ")")
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if isElement(n, "p") || isElement(n, "div") || isElement(n, "li") {
			sb.WriteString("\n")
		}
	}
	walk(n)

	// Text nodes carry the source HTML's own indentation (tabs before each
	// line), which isn't meaningful formatting - just pretty-printing of
	// the markup - so strip it per line rather than passing it through.
	lines := strings.Split(sb.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
