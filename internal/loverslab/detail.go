package loverslab

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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
	Title string
	// Description is the JSON-LD block's own flattened, plain-text rendering of the
	// description - real newlines, but every paragraph break, embedded image, and
	// bit of formatting (bold/italic/underline/links) the real page actually has is
	// gone, along with a lot of stray blank lines (the source page uses plenty of
	// genuinely empty paragraphs purely for vertical spacing, which flattening turns
	// into blank lines here). Kept only as a plain-text fallback; DescriptionBlocks
	// below is what the detail view actually renders.
	Description string
	// DescriptionBlocks is the same description, parsed instead from the page's own
	// rich-text DOM (data-controller="core.front.core.lightboxedImages") - real
	// paragraphs (empty ones dropped), embedded images, links, and basic emphasis,
	// in the order they appear. Empty when that container wasn't found for some
	// reason (Description above still covers that case).
	DescriptionBlocks []DescriptionBlock
	Version           string // the author's own declared version string
	FileSize          string // as displayed by the site, e.g. "111.41 MB"
	Author            FileAuthor
	Screenshots       []Screenshot
	Views             int
	Downloads         int
	// DateModified is the site's own ISO 8601 "dateModified" timestamp, kept as the
	// raw string (not parsed into a time.Time) the same way every other site-supplied
	// string in this package is trusted as-is - a real timestamp, unlike the category
	// listing's own display string (FileSummary.Updated, e.g. "2 days ago"), so this
	// is what the update check compares against a saved install's own value, not that
	// display string (see internal/loverslabtracking.Entry.InstalledDateModified).
	DateModified string
}

// DescriptionBlock is one paragraph of a Downloads file's real description, in the
// order it appears on the real page - either a single embedded image or formatted
// text, never both. A paragraph with only whitespace (the source page uses plenty
// of those purely as spacing) never becomes a block at all.
type DescriptionBlock struct {
	// ImageURL is set for an image block; Runs is empty for one.
	ImageURL string
	// Runs is set for a text block, broken up so each run can carry its own
	// bold/italic/underline/link state without resorting to raw HTML - safe to
	// render directly, never dangerouslySetInnerHTML of a third party's markup.
	Runs []DescriptionRun
	// Heading is 1-6 for a Markdown ATX heading ("# Title" through "######
	// Title"), 0 otherwise. Divider is true for a paragraph that's just "---"
	// or "***" on its own. Quote is true for a line starting with "> ".
	// ListItem is true for a real HTML <li>, or a plain-text line starting
	// with "* "/"- " that was never wrapped in one.
	//
	// These all come from literally-typed Markdown syntax some authors paste
	// straight into the rich-text editor as plain text rather than using its
	// own formatting toolbar - confirmed live against a real Downloads file
	// (https://www.loverslab.com/files/file/49547-the-knights-of-the-brothel-
	// english-translation/) whose entire "About This File" is written this
	// way ("# Mod Translation...", "## Mod Description", "> ⚠️ **Warning:**",
	// "* **The Grand Master:** ...", "---"), all landing as inert plain text
	// otherwise - real HTML tags (<strong>, <a>, ...) already have their own
	// handling and are never double-processed here.
	Heading  int
	Divider  bool
	Quote    bool
	ListItem bool
	// QuotedAuthor is set for a real forum "Quote" of another post
	// (<blockquote class="ipsQuote">, confirmed live on a real topic reply) -
	// who is being quoted (its own data-ipsquote-username), with QuotedBlocks
	// holding the quoted excerpt's own content, parsed the same way as
	// everything else (so a quote containing its own bold/links/images still
	// renders correctly). Distinct from the plain Quote flag above, which is
	// just literal "> " Markdown text with no real attribution behind it.
	QuotedAuthor string
	QuotedBlocks []DescriptionBlock
	// EmbedURL is set for a real embedded reference to another LoversLab
	// post/topic (<iframe data-controller="core.front.core.autosizeiframe">,
	// confirmed live - IPS's own rich preview when a member pastes a link to
	// another topic or comment). Rendering the iframe itself isn't practical
	// here (it needs the site's own session/JS to render anything at all, and
	// this app's webview has no reason to load third-party pages inside
	// itself), so this is surfaced as a real, clickable reference to open in
	// the system browser instead of silently dropped or shown as a dead gray
	// box.
	EmbedURL string
}

// DescriptionRun is one contiguous span of a text DescriptionBlock sharing the
// same formatting.
type DescriptionRun struct {
	Text      string
	Bold      bool
	Italic    bool
	Underline bool
	// LinkURL is set when this run is a hyperlink - opened in the system browser,
	// the same way every other external link in this app is.
	LinkURL string
	// EmoteURL is set when this run is a real inline emoticon (a plain <img
	// data-emoticon> the site's own editor inserts for e.g. ":D" - confirmed
	// live) rather than text - Text is empty and EmoteAlt carries its own real
	// alt text (the typed shortcode, e.g. ":D") for a moment, never both.
	// Kept as its own small inline run rather than a block-level
	// DescriptionBlock.ImageURL: a real embedded screenshot is a photo-sized
	// block of its own, an emoticon is a tiny icon that belongs inline with
	// the sentence around it - conflating the two is exactly why an emoticon
	// used to render "massive," at full description-image size.
	EmoteURL string
	EmoteAlt string
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
			// Both start as a real, non-nil empty slice rather than each field's
			// own zero value (nil) - a nil Go slice marshals to JSON null, not
			// [], and the frontend never expects to see null for either of
			// these (a file with zero screenshots, or one whose description
			// body simply wasn't found, is common, not an error).
			Screenshots:       []Screenshot{},
			DescriptionBlocks: []DescriptionBlock{},
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
		if body := findOne(doc, isDescriptionBody); body != nil {
			detail.DescriptionBlocks = parseDescriptionBlocks(body)
		}
		return detail, true
	}
	return FileDetail{}, false
}

// isDescriptionBody finds "About This File"'s own rich-text container - the
// controller name IPS gives any block that can contain lightbox-able embedded
// images, confirmed unique on a real file page (unlike ipsType_richText alone,
// which every rich-text block on the site shares).
func isDescriptionBody(n *html.Node) bool {
	return attrOr(n, "data-controller") == "core.front.core.lightboxedImages"
}

// descriptionBuilder walks a description's rich-text DOM into DescriptionBlocks -
// pulled out from parseDescriptionBlocks so its own recursive walk can carry
// state (the current text block's accumulated runs) without a package-level var.
type descriptionBuilder struct {
	blocks []DescriptionBlock
	runs   []DescriptionRun
	// skipAttachLinks is true for a forum post's own body (see
	// parsePostContentBlocks) - an attachment link is dropped entirely rather
	// than rendered as a normal link, since the caller already surfaces every
	// attachment separately as its own chip (ListTopicPosts' own
	// Post.Attachments); showing it again here would just be noise. Never set
	// for a Downloads file's description or changelog, neither of which has
	// this concept at all.
	skipAttachLinks bool
}

// parseDescriptionBlocks is GetFileDetail's (and ChangelogEntry's) own rich-text
// parsing, pulled out so it can be tested directly against a hand-built
// fragment instead of a real request - see detail_test.go.
func parseDescriptionBlocks(body *html.Node) []DescriptionBlock {
	return parseRichBlocks(body, false)
}

// parsePostContentBlocks is a forum post's own rich-text parsing - the exact
// same real Markdown/formatting/quote support as parseDescriptionBlocks,
// minus attachment links (see skipAttachLinks above).
func parsePostContentBlocks(body *html.Node) []DescriptionBlock {
	return parseRichBlocks(body, true)
}

func parseRichBlocks(body *html.Node, skipAttachLinks bool) []DescriptionBlock {
	// Never nil, even if the body turns out to hold nothing but spacer
	// paragraphs - see parseFileDetail's own comment on why that distinction
	// matters once this crosses the JSON wire.
	b := &descriptionBuilder{blocks: []DescriptionBlock{}, skipAttachLinks: skipAttachLinks}
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		b.walk(c, DescriptionRun{})
	}
	b.flushText(false)
	return b.blocks
}

// walk recurses through n, carrying the formatting (bold/italic/underline/
// LinkURL) accumulated from its own ancestors so far - style is only ever added
// going down the tree, never removed, matching how nested <strong><em> markup
// actually composes.
func (b *descriptionBuilder) walk(n *html.Node, style DescriptionRun) {
	switch {
	case n.Type == html.TextNode:
		style.Text = collapseWhitespace(n.Data)
		b.runs = append(b.runs, style)
		return
	case isElement(n, "img") && hasAttr(n, "data-emoticon"):
		// A real inline emoticon (see DescriptionRun.EmoteURL) - stays part of
		// the current text block's own runs, never its own block, so it flows
		// with the sentence around it at icon size instead of at full
		// description-image size.
		style.Text = ""
		style.EmoteURL = attrOr(n, "src")
		style.EmoteAlt = attrOr(n, "alt")
		b.runs = append(b.runs, style)
		return
	case isElement(n, "img"):
		b.flushText(false)
		if src := attrOr(n, "src"); src != "" {
			b.blocks = append(b.blocks, DescriptionBlock{ImageURL: src})
		}
		return
	case isElement(n, "br"):
		style.Text = "\n"
		b.runs = append(b.runs, style)
		return
	case isElement(n, "a") && b.skipAttachLinks && hasClass(n, "ipsAttachLink"):
		// A real attached image still shows inline, the same as any other
		// embedded image (confirmed live: a post's own attached screenshot
		// was never rendered anywhere, just named in its separate attachment
		// chip) - only a non-image attachment (a zip, a log file) is skipped
		// here, since that chip is the only sensible way to show it at all.
		if hasClass(n, "ipsAttachLink_image") {
			if img := findOne(n, func(c *html.Node) bool { return isElement(c, "img") }); img != nil {
				if src := attrOr(img, "src"); src != "" {
					b.flushText(false)
					b.blocks = append(b.blocks, DescriptionBlock{ImageURL: src})
				}
			}
		}
		return
	case isElement(n, "iframe") && hasAttr(n, "data-embedid"):
		b.flushText(false)
		if src := attrOr(n, "src"); src != "" {
			b.blocks = append(b.blocks, DescriptionBlock{EmbedURL: src})
		}
		return
	case isElement(n, "blockquote") && hasClass(n, "ipsQuote"):
		b.flushText(false)
		var quotedBlocks []DescriptionBlock
		if contents := findOne(n, func(c *html.Node) bool { return isElement(c, "div") && hasClass(c, "ipsQuote_contents") }); contents != nil {
			quotedBlocks = parseRichBlocks(contents, b.skipAttachLinks)
		}
		b.blocks = append(b.blocks, DescriptionBlock{
			QuotedAuthor: attrOr(n, "data-ipsquote-username"),
			QuotedBlocks: quotedBlocks,
		})
		return
	}

	switch {
	case isElement(n, "strong") || isElement(n, "b"):
		style.Bold = true
	case isElement(n, "em") || isElement(n, "i"):
		style.Italic = true
	case isElement(n, "u"):
		style.Underline = true
	case isElement(n, "a"):
		if href := attrOr(n, "href"); href != "" {
			style.LinkURL = href
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.walk(c, style)
	}
	switch {
	case isElement(n, "li"):
		b.flushText(true)
	case isElement(n, "p") || isElement(n, "div"):
		b.flushText(false)
	}
}

// markdownHeadingPattern, markdownQuotePattern, markdownListPattern and
// markdownDividerPattern match literal Markdown syntax at the very start of a
// block's own first run - see DescriptionBlock's own comment on why plain text
// ever needs this at all.
var (
	markdownHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	markdownQuotePattern   = regexp.MustCompile(`^>\s?(.*)$`)
	markdownListPattern    = regexp.MustCompile(`^[*-]\s+(.*)$`)
	markdownDividerPattern = regexp.MustCompile(`^(-{3,}|\*{3,})$`)
	// markdownBoldPattern matches inline "**bold**" within a run's own text -
	// non-greedy, so "**a** and **b**" splits into two separate bold spans
	// rather than one run spanning "a** and **b".
	markdownBoldPattern = regexp.MustCompile(`\*\*(.+?)\*\*`)
)

// flushText closes off the text block accumulated so far, dropping it entirely
// if nothing but whitespace (including the source page's own literal " "
// non-breaking spaces) ever made it in - this is what stops the source page's
// habit of empty <p>&nbsp;</p> spacer paragraphs from becoming blank lines here;
// this app lays the text out with its own spacing instead. isListItem is true
// when this block came from a real <li>, which already settles ListItem
// without needing the plain-text "* "/"- " marker check below.
func (b *descriptionBuilder) flushText(isListItem bool) {
	runs := trimRuns(b.runs)
	b.runs = nil
	if len(runs) == 0 {
		return
	}

	if len(runs) == 1 && markdownDividerPattern.MatchString(strings.TrimSpace(runs[0].Text)) {
		b.blocks = append(b.blocks, DescriptionBlock{Divider: true})
		return
	}

	block := DescriptionBlock{ListItem: isListItem}
	first := strings.TrimLeft(runs[0].Text, " ")
	switch {
	case markdownHeadingPattern.MatchString(first):
		m := markdownHeadingPattern.FindStringSubmatch(first)
		block.Heading = len(m[1])
		runs[0].Text = m[2]
	case markdownQuotePattern.MatchString(first):
		m := markdownQuotePattern.FindStringSubmatch(first)
		block.Quote = true
		runs[0].Text = m[1]
	case !isListItem && markdownListPattern.MatchString(first):
		m := markdownListPattern.FindStringSubmatch(first)
		block.ListItem = true
		runs[0].Text = m[1]
	}

	block.Runs = expandMarkdownBold(runs)
	b.blocks = append(b.blocks, block)
}

// expandMarkdownBold splits any "**bold**" markdown inside a run's own plain
// text into its own separate, actually-bold run - a run that's already a link
// is left alone (its label is never split mid-link).
func expandMarkdownBold(runs []DescriptionRun) []DescriptionRun {
	out := make([]DescriptionRun, 0, len(runs))
	for _, r := range runs {
		if r.LinkURL != "" || !strings.Contains(r.Text, "**") {
			out = append(out, r)
			continue
		}
		last := 0
		for _, m := range markdownBoldPattern.FindAllStringSubmatchIndex(r.Text, -1) {
			if m[0] > last {
				plain := r
				plain.Text = r.Text[last:m[0]]
				out = append(out, plain)
			}
			bold := r
			bold.Bold = true
			bold.Text = r.Text[m[2]:m[3]]
			out = append(out, bold)
			last = m[1]
		}
		if last < len(r.Text) {
			rest := r
			rest.Text = r.Text[last:]
			out = append(out, rest)
		}
	}
	return out
}

// collapseWhitespace mirrors how a real browser renders ordinary flowed text: a
// text node's own line breaks and indentation are never semantic - real line
// breaks come from <br>, which already gets its own dedicated "\n" run in walk -
// so any run of newlines/tabs/spaces inside one text node collapses to a single
// space, exactly like CSS's default white-space: normal.
func collapseWhitespace(s string) string {
	var sb strings.Builder
	lastSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !lastSpace {
				sb.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		sb.WriteRune(r)
		lastSpace = false
	}
	return sb.String()
}

// trimRuns drops leading/trailing whitespace-only runs, and trims incidental
// leading/trailing whitespace off the first/last surviving run - the source
// page's own pretty-printed markup routinely opens and closes a real paragraph
// with a blank text node that's just the HTML's own indentation, never
// deliberate spacing. Returns nil if nothing but whitespace survives at all.
func trimRuns(runs []DescriptionRun) []DescriptionRun {
	// A real inline emoticon run (see DescriptionRun.EmoteURL) has no text of
	// its own at all - never mistake that for whitespace-only blankness, or a
	// message that's just a leading/trailing emoji loses it entirely.
	isBlank := func(r DescriptionRun) bool {
		return r.EmoteURL == "" && strings.TrimSpace(strings.ReplaceAll(r.Text, " ", " ")) == ""
	}
	start := 0
	for start < len(runs) && isBlank(runs[start]) {
		start++
	}
	end := len(runs)
	for end > start && isBlank(runs[end-1]) {
		end--
	}
	if start >= end {
		return nil
	}
	out := append([]DescriptionRun{}, runs[start:end]...)
	out[0].Text = strings.TrimLeft(out[0].Text, " \t\r\n")
	out[len(out)-1].Text = strings.TrimRight(out[len(out)-1].Text, " \t\r\n")
	return out
}
