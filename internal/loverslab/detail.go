package loverslab

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/net/html"
)

// Screenshot is one image on a file's detail page - both the full-size original and
// its thumbnail, since they're genuinely separate stored files on LoversLab's own CDN
// (see docs/loverslab.md's Media section), not one derived from the other by URL.
type Screenshot struct {
	URL          string
	ThumbnailURL string
}

// FileAuthor is a Downloads file's uploader, as shown on its own detail page.
type FileAuthor struct {
	Name     string
	URL      string
	ImageURL string
}

// FileDetail is a Downloads file's own page: its full description and screenshot
// gallery, for the Browse tab's mod detail view - see GetFileDetail.
type FileDetail struct {
	Title       string
	Description string // plain text with real newlines - not HTML, no richText() needed
	Version     string // the author's own declared version string
	FileSize    string // as displayed by the site, e.g. "111.41 MB"
	Author      FileAuthor
	Screenshots []Screenshot
	Views       int
	Downloads   int
	// DateModified is the site's own ISO 8601 "dateModified" timestamp, kept as the
	// raw string (not parsed into a time.Time) the same way every other site-supplied
	// string in this package is trusted as-is - a real timestamp, unlike the category
	// listing's own display string (FileSummary.Updated, e.g. "2 days ago"), so this
	// is what the update check compares against a saved install's own value, not that
	// display string (see internal/loverslabtracking.Entry.InstalledDateModified).
	DateModified string
}

// webApplicationLD mirrors the schema.org WebApplication JSON-LD block every
// Downloads file page embeds (the first application/ld+json script tag whose @type is
// "WebApplication" - later ones on the same page are site-wide boilerplate: WebSite,
// Organization, BreadcrumbList, ContactPage). See docs/loverslab.md's "A file's detail
// page" section for how this was found (confirmed live against three real files) and
// why it's used instead of DOM-parsing the description prose or the screenshot
// carousel's data-fullurl attributes - everything needed is already here, structured.
type webApplicationLD struct {
	Type            string `json:"@type"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	SoftwareVersion string `json:"softwareVersion"`
	FileSize        string `json:"fileSize"`
	Author          struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Image string `json:"image"`
	} `json:"author"`
	InteractionStatistic []struct {
		InteractionType      string `json:"interactionType"`
		UserInteractionCount int    `json:"userInteractionCount"`
	} `json:"interactionStatistic"`
	DateModified string `json:"dateModified"`
	Screenshot   []struct {
		URL       string `json:"url"`
		Thumbnail struct {
			URL string `json:"url"`
		} `json:"thumbnail"`
	} `json:"screenshot"`
}

// GetFileDetail fetches a Downloads file's own page and returns its full description
// and screenshot gallery.
func (c *Client) GetFileDetail(ctx context.Context, filePageURL string) (FileDetail, error) {
	doc, err := c.getDocument(ctx, filePageURL)
	if err != nil {
		return FileDetail{}, fmt.Errorf("getting file detail: %w", err)
	}
	detail, ok := parseFileDetail(doc)
	if !ok {
		return FileDetail{}, fmt.Errorf("getting file detail: no WebApplication data found on %s", filePageURL)
	}
	return detail, nil
}

// parseFileDetail is GetFileDetail's own parsing, pulled out so it can be tested
// directly against a hand-built document instead of a real request - see
// detail_test.go.
func parseFileDetail(doc *html.Node) (FileDetail, bool) {
	scripts := find(doc, func(n *html.Node) bool {
		return isElement(n, "script") && attrOr(n, "type") == "application/ld+json"
	})
	for _, script := range scripts {
		var ld webApplicationLD
		if err := json.Unmarshal([]byte(text(script)), &ld); err != nil {
			continue // site-wide boilerplate blocks aren't all the same shape
		}
		if ld.Type != "WebApplication" {
			continue
		}

		detail := FileDetail{
			Title:        ld.Name,
			Description:  ld.Description,
			Version:      ld.SoftwareVersion,
			FileSize:     ld.FileSize,
			Author:       FileAuthor{Name: ld.Author.Name, URL: ld.Author.URL, ImageURL: ld.Author.Image},
			DateModified: ld.DateModified,
		}
		for _, s := range ld.InteractionStatistic {
			switch s.InteractionType {
			case "http://schema.org/ViewAction":
				detail.Views = s.UserInteractionCount
			case "http://schema.org/DownloadAction":
				detail.Downloads = s.UserInteractionCount
			}
		}
		for _, s := range ld.Screenshot {
			detail.Screenshots = append(detail.Screenshots, Screenshot{URL: s.URL, ThumbnailURL: s.Thumbnail.URL})
		}
		return detail, true
	}
	return FileDetail{}, false
}
