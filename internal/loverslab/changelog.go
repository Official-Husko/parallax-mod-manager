package loverslab

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// ChangelogEntry is one version's release notes, as shown in a Downloads
// file's "What's New in Version X" section.
type ChangelogEntry struct {
	Version     string
	Released    string // as displayed by the site, e.g. "August 15"
	Description string
}

func isChangelogSection(n *html.Node) bool {
	return isElement(n, "section") && attrOr(n, "data-controller") == "downloads.front.view.changeLog"
}

// ListChangelog returns a file's release notes, newest first. Most files
// only have one entry (whatever the author wrote for the current upload,
// which is all IPS keeps visible unless the author submitted distinct
// versions), and many have none at all - the changelog is optional, so a
// nil, nil result just means this file never had one written for it.
func (c *Client) ListChangelog(ctx context.Context, filePageURL string) ([]ChangelogEntry, error) {
	doc, err := c.getDocument(ctx, filePageURL)
	if err != nil {
		return nil, fmt.Errorf("listing changelog: %w", err)
	}

	section := findOne(doc, isChangelogSection)
	if section == nil {
		return nil, nil
	}

	entries := make([]ChangelogEntry, 0, 1)
	if entry, ok := parseChangelogSection(section); ok {
		entries = append(entries, entry)
	}

	// Files with multiple retained versions show a dropdown of them; the
	// page we already fetched is whichever one is "?changelog=0", so only
	// the other entries need a further request.
	versionLinks := find(section, func(n *html.Node) bool {
		return isElement(n, "a") && strings.Contains(attrOr(n, "href"), "changelog=") && !strings.HasSuffix(attrOr(n, "href"), "changelog=0")
	})
	for _, link := range versionLinks {
		pageDoc, err := c.getDocument(ctx, attrOr(link, "href"))
		if err != nil {
			return entries, fmt.Errorf("listing changelog: %w", err)
		}
		sec := findOne(pageDoc, isChangelogSection)
		if sec == nil {
			continue
		}
		if entry, ok := parseChangelogSection(sec); ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func parseChangelogSection(section *html.Node) (ChangelogEntry, bool) {
	dataNode := findOne(section, func(n *html.Node) bool { return attrOr(n, "data-role") == "changeLogData" })
	if dataNode == nil {
		return ChangelogEntry{}, false
	}

	version := ""
	if v := findOne(section, func(n *html.Node) bool { return attrOr(n, "data-role") == "versionTitle" }); v != nil {
		version = strings.TrimSpace(text(v))
	}

	released := ""
	if t := findOne(dataNode, func(n *html.Node) bool { return isElement(n, "time") }); t != nil {
		released = strings.TrimSpace(text(t))
	}

	description := ""
	if rt := findOne(dataNode, func(n *html.Node) bool { return isElement(n, "div") && hasClass(n, "ipsType_richText") }); rt != nil {
		description = richText(rt)
	}

	return ChangelogEntry{Version: version, Released: released, Description: description}, true
}
