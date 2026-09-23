package loverslab

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Category is a Downloads section, e.g. "Skyrim: Special Edition" (a
// top-level game section) or "Regular Mods" (one of its subcategories).
type Category struct {
	ID    int
	Name  string
	URL   string
	Files int // item count shown next to it in the sidebar
	Depth int // 0 = top-level section, 1 = subcategory
}

var categoryIDPattern = regexp.MustCompile(`/files/category/(\d+)-`)

// ListCategories returns the site-wide Downloads category tree (every top-level section and
// the subcategories it shows at that level), as shown in the sidebar of the root /files/ page.
// IPS4's sidebar only expands a category's own deeper children once that category is the one
// being viewed - see ListSubcategories for those.
func (c *Client) ListCategories(ctx context.Context) ([]Category, error) {
	return c.listCategoriesAt(ctx, BaseURL+"/files/")
}

// ListSubcategories returns the category tree as shown when viewing categoryURL's own page -
// the same sidebar ListCategories reads, but rooted at that category instead of the site-wide
// root, which is how a category's own deeper children (not shown in ListCategories' own
// result) become visible at all.
func (c *Client) ListSubcategories(ctx context.Context, categoryURL string) ([]Category, error) {
	return c.listCategoriesAt(ctx, categoryURL)
}

func (c *Client) listCategoriesAt(ctx context.Context, pageURL string) ([]Category, error) {
	doc, err := c.getDocument(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("listing categories: %w", err)
	}
	return parseCategories(doc), nil
}

// parseCategories is ListCategories' own parsing, pulled out so it can be
// tested directly against a hand-built document instead of a real request -
// see browse_test.go.
func parseCategories(doc *html.Node) []Category {
	links := find(doc, func(n *html.Node) bool {
		return isElement(n, "a") && hasClass(n, "ipsSideMenu_item")
	})

	categories := make([]Category, 0, len(links))
	for _, a := range links {
		// The sidebar's "(and N more)" expanders share the ipsSideMenu_item
		// class and a real category's href, but carry no count badge -
		// that's how we tell them apart from actual categories.
		countNode := findOne(a, func(n *html.Node) bool { return hasClass(n, "cDownloadsCategoryCount") })
		if countNode == nil {
			continue
		}

		href := attrOr(a, "href")
		countText := strings.TrimSpace(text(countNode))
		count, _ := strconv.Atoi(countText)

		// The count and the name are adjacent text/elements in document
		// order (count always first) regardless of which tag holds which,
		// so stripping the count's own text as a prefix reliably isolates
		// the name.
		name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text(a)), countText))

		id := 0
		if m := categoryIDPattern.FindStringSubmatch(href); m != nil {
			id, _ = strconv.Atoi(m[1])
		}

		categories = append(categories, Category{
			ID:    id,
			Name:  name,
			URL:   href,
			Files: count,
			Depth: sideMenuDepth(a),
		})
	}
	return categories
}

// sideMenuDepth counts nested ipsSideMenu_list ancestors, which is how the
// site distinguishes a top-level section from its subcategories. The
// outermost ipsSideMenu_list is the sidebar's own root list, so it's
// subtracted off to make top-level sections come out at depth 0.
func sideMenuDepth(n *html.Node) int {
	depth := 0
	for p := n.Parent; p != nil; p = p.Parent {
		if isElement(p, "ul") && hasClass(p, "ipsSideMenu_list") {
			depth++
		}
	}
	if depth > 0 {
		depth--
	}
	return depth
}

// FileSummary is one entry in a category's file listing.
type FileSummary struct {
	ID           int
	Title        string
	URL          string
	Author       string
	AuthorURL    string
	Updated      string // as displayed by the site, e.g. "Sunday at 03:07 AM"
	ThumbnailURL string // static.loverslab.com CDN image, empty if the file has no screenshot
}

var fileIDPattern = regexp.MustCompile(`/files/file/(\d+)-`)

// ListFiles returns the files on one (1-indexed) page of a category
// listing, plus the total number of pages available.
func (c *Client) ListFiles(ctx context.Context, categoryURL string, page int) ([]FileSummary, int, error) {
	pageURL := categoryURL
	if page > 1 {
		pageURL = strings.TrimRight(categoryURL, "/") + fmt.Sprintf("/page/%d/", page)
	}

	doc, err := c.getDocument(ctx, pageURL)
	if err != nil {
		return nil, 0, fmt.Errorf("listing files: %w", err)
	}
	files, totalPages := parseFileListing(doc)
	return files, totalPages, nil
}

// parseFileListing is ListFiles' own parsing, pulled out so it can be
// tested directly against a hand-built document instead of a real request -
// see browse_test.go.
func parseFileListing(doc *html.Node) ([]FileSummary, int) {
	totalPages := 1
	if pag := findOne(doc, func(n *html.Node) bool { return isElement(n, "ul") && hasClass(n, "ipsPagination") }); pag != nil {
		if v := attrOr(pag, "data-pages"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				totalPages = n
			}
		}
	}

	items := find(doc, func(n *html.Node) bool { return isElement(n, "li") && hasClass(n, "ipsDataItem") })

	files := make([]FileSummary, 0, len(items))
	for _, item := range items {
		// The title's own <h4> used to carry an "ipsDataItem_title" class
		// (confirmed live it no longer does - the site dropped it from this
		// listing's markup at some point, silently emptying every listing
		// page's results since nothing else here ever matched), so the title
		// is found directly by its own wrapping span instead of gating on
		// that heading class first. A file with a "prefix" tag (e.g.
		// "immersion") renders that tag as its own <a> right before the real
		// title, in a plain <span> with neither of this span's two classes,
		// so it's never mistaken for the title itself.
		titleSpan := findOne(item, func(n *html.Node) bool {
			return isElement(n, "span") && hasClass(n, "ipsType_break") && hasClass(n, "ipsContained")
		})
		if titleSpan == nil {
			continue
		}
		titleLink := findOne(titleSpan, func(n *html.Node) bool { return isElement(n, "a") })
		if titleLink == nil {
			continue
		}
		href := attrOr(titleLink, "href")
		title := strings.TrimSpace(text(titleLink))

		var author, authorURL string
		if a := findOne(item, func(n *html.Node) bool {
			return isElement(n, "a") && strings.Contains(attrOr(n, "href"), "/profile/")
		}); a != nil {
			authorURL = attrOr(a, "href")
			author = strings.TrimSpace(text(a))
		}

		updated := ""
		if t := findOne(item, func(n *html.Node) bool { return isElement(n, "time") }); t != nil {
			updated = strings.TrimSpace(text(t))
		}

		thumbnailURL := ""
		if thumb := findOne(item, func(n *html.Node) bool { return isElement(n, "a") && hasClass(n, "ipsThumb") }); thumb != nil {
			if img := findOne(thumb, func(n *html.Node) bool { return isElement(n, "img") }); img != nil {
				thumbnailURL = attrOr(img, "src")
			}
		}

		id := 0
		if m := fileIDPattern.FindStringSubmatch(href); m != nil {
			id, _ = strconv.Atoi(m[1])
		}

		files = append(files, FileSummary{
			ID:           id,
			Title:        title,
			URL:          href,
			Author:       author,
			AuthorURL:    authorURL,
			Updated:      updated,
			ThumbnailURL: thumbnailURL,
		})
	}
	return files, totalPages
}

// SupportTopicURL returns the URL of a Downloads file's linked "Get Support" forum
// topic, or "" if it has none - plenty of files don't link one at all. Files on this
// site don't have native comments (see docs/loverslab.md's Comments section); this
// linked topic is the closest thing, read by ListFileSupportPosts.
func (c *Client) SupportTopicURL(ctx context.Context, filePageURL string) (string, error) {
	doc, err := c.getDocument(ctx, filePageURL)
	if err != nil {
		return "", fmt.Errorf("finding support topic: %w", err)
	}
	return parseSupportTopicURL(doc), nil
}

// parseSupportTopicURL is SupportTopicURL's own parsing, pulled out so it can be
// tested directly against a hand-built document instead of a real request - see
// topics_test.go.
func parseSupportTopicURL(doc *html.Node) string {
	link := findOne(doc, func(n *html.Node) bool {
		return isElement(n, "a") && attrOr(n, "title") == "Get support for this download"
	})
	if link == nil {
		return ""
	}
	return attrOr(link, "href")
}
