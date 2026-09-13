package steamapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// changelogURL is a var, not a const, so tests can point it at a local
// httptest.Server. %s is the published file id.
var changelogURL = "https://steamcommunity.com/sharedfiles/filedetails/changelog/%s"

// ChangelogEntry is one real Steam Workshop update note, scraped from the
// item's own changelog page - see docs/steam-web-api.md. There's no
// public API for this (unlike GetPublishedFileDetails/GetAppDetails
// above), so this is the only Steam data this project reads by parsing
// real HTML rather than a documented JSON/XML response - deliberately
// scoped to the changelog page's first page only (confirmed to hold the
// 10 most recent entries), since crawling a mod's full update history
// would mean many more requests for what's meant to be a preview.
type ChangelogEntry struct {
	// Headline is Steam's own wording verbatim (e.g. "Update: 3 May @
	// 7:41am") - never reparsed into a machine timestamp: the exact
	// format is undocumented and confirmed to drop the year for the
	// current year (checked against a real multi-year update history),
	// so a naive parse would misdate every recent entry every January.
	Headline         string
	Author           string
	AuthorProfileURL string
	// Body is a readable plain-text rendering of the entry's real update
	// notes (block tags and <br> become newlines, everything else is
	// flattened) - the source is real HTML, not BBCode like
	// PublishedFileDetails.Description, so it needs its own conversion
	// rather than reusing the frontend's stripBBCode. Empty is a normal,
	// real outcome (a version bump with no written notes).
	Body string
}

// GetChangelog fetches publishedFileID's real, most recent Workshop update
// notes. Returns an empty, non-nil slice (not an error) if the page has no
// changelog entries at all - a mod never updated since upload is a normal
// outcome, not a failure.
func GetChangelog(ctx context.Context, publishedFileID string) ([]ChangelogEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(changelogURL, publishedFileID), nil)
	if err != nil {
		return nil, fmt.Errorf("steamapi: building request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steamapi: requesting changelog for %s: %w", publishedFileID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steamapi: changelog request for %s failed: HTTP %d", publishedFileID, resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("steamapi: parsing changelog for %s: %w", publishedFileID, err)
	}

	entries := []ChangelogEntry{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "changeLogCtn") {
			entries = append(entries, parseChangelogEntry(n))
			return // a changeLogCtn div never nests another one
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return entries, nil
}

// parseChangelogEntry reads one real <div class="... changeLogCtn"> block.
//
// A real, confirmed quirk shapes this: the page's own markup nests a
// <div> directly inside the body's <p id="..."> tag, which is invalid
// HTML5 - a spec-compliant parser (this one included, matching what a
// real browser does) auto-closes that <p> empty right where the <div>
// starts. The real body content then lands as further siblings of that
// now-empty <p>, still inside the same changeLogCtn div, rather than
// inside it. Confirmed directly by dumping the parsed tree against a real
// changelog page, both for an entry with real body text and for one with
// none. So the body is collected as everything between the author div and
// the comments-link div (or the end of the entry, if there's no comments
// link) - never by looking for a <p> to hold it.
func parseChangelogEntry(div *html.Node) ChangelogEntry {
	var entry ChangelogEntry
	var body strings.Builder
	collecting := false
	for c := div.FirstChild; c != nil; c = c.NextSibling {
		// The real body is a flat run of text and <br> nodes sitting
		// directly under div (see the doc comment) - only element nodes
		// can be one of the known structural pieces below, so a plain
		// text node always falls through to the body collection at the
		// bottom, same as everything else once collecting is true.
		if c.Type == html.ElementNode {
			switch {
			case c.Data == "div" && hasClass(c, "headline"):
				entry.Headline = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(textContent(c)), "Update:"))
				continue
			case c.Data == "div" && hasClass(c, "author"):
				if a := findFirst(c, "a"); a != nil {
					entry.Author = strings.TrimSpace(textContent(a))
					entry.AuthorProfileURL = attr(a, "href")
				}
				collecting = true
				continue
			case c.Data == "div" && hasClass(c, "commentsLink"):
				collecting = false
				continue
			case c.Data == "p":
				// The empty artifact left behind by the auto-close
				// described above - never holds real content.
				continue
			}
		}
		if collecting {
			writeText(&body, c)
		}
	}
	entry.Body = normalizeBlankLines(body.String())
	return entry
}

// hasClass reports whether n's class attribute contains class as one of
// its space-separated words.
func hasClass(n *html.Node, class string) bool {
	for _, word := range strings.Fields(attr(n, "class")) {
		if word == class {
			return true
		}
	}
	return false
}

// attr returns n's value for the given attribute, or "" if absent.
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// findFirst returns the first descendant of n (n itself included) with
// the given tag name, or nil if there is none.
func findFirst(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirst(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// textContent concatenates every text node under n. Returns "" for a nil
// n so callers don't need their own nil check first.
func textContent(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// blockTags renders as a line break in writeText's plain-text conversion -
// the real changelog bodies checked only ever use div/p for structure and
// <br> for line breaks, but this covers the other block-level tags Steam's
// editor could plausibly emit too.
var blockTags = map[string]bool{
	"div": true, "p": true, "li": true, "ul": true, "ol": true,
	"h1": true, "h2": true, "h3": true, "blockquote": true,
}

// writeText renders n (and its children) into b as readable plain text: a
// <br> or the end of a block-level element becomes a newline, everything
// else is flattened inline. Not a general HTML-to-text converter - just
// enough for the real markup Steam's own changelog editor produces.
func writeText(b *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode && n.Data == "br" {
		b.WriteString("\n")
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeText(b, c)
	}
	if n.Type == html.ElementNode && blockTags[n.Data] {
		b.WriteString("\n")
	}
}

// normalizeBlankLines trims each line and collapses three or more
// consecutive newlines (a real, common result of "<br><br>" between
// paragraphs sitting next to a block element's own trailing newline) down
// to a single blank line between paragraphs.
func normalizeBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	blank := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
