package loverslab

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Trimmed from a real file page (Stable Portraits, fetched 2026-09-23 - see
// docs/loverslab.md's "A file's detail page" section), including a couple of the
// site-wide boilerplate JSON-LD blocks that appear later on every page, to confirm
// those are correctly skipped rather than mistaken for the real one.
const fileDetailFixture = `<html><head>
<script type='application/ld+json'>
{
    "@context": "http://schema.org",
    "@type": "WebApplication",
    "url": "https://www.loverslab.com/files/file/31347-stable-portraits/",
    "name": "Stable Portraits",
    "description": "Hello,\n \n\tThis is an animated portrait pack.\n \n\t132 Elf Portraits.",
    "applicationCategory": "Stellaris",
    "downloadUrl": "https://www.loverslab.com/files/file/31347-stable-portraits/?do=download",
    "dateCreated": "2024-01-10T18:09:13+0100",
    "fileSize": "111.41 MB",
    "softwareVersion": "v2",
    "author": {
        "@type": "Person",
        "name": "Karsus",
        "image": "https://static.loverslab.com/uploads/profiles/profile/photo-thumb-993535.jpg",
        "url": "https://www.loverslab.com/profile/993535-karsus/"
    },
    "interactionStatistic": [
        {"@type": "InteractionCounter", "interactionType": "http://schema.org/ViewAction", "userInteractionCount": 33402},
        {"@type": "InteractionCounter", "interactionType": "http://schema.org/DownloadAction", "userInteractionCount": 6049}
    ],
    "dateModified": "2026-09-13T20:01:52+0200",
    "screenshot": [
        {
            "@type": "ImageObject",
            "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00066-4195690853.png.dd8fe7e146d4ba158b92ae814053e1eb.png",
            "thumbnail": {"@type": "ImageObject", "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00066-4195690853.thumb.png.f9184ebbcff34d5673a6f9e7c3579498.png"}
        },
        {
            "@type": "ImageObject",
            "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00115-4146571628.png.38c1757839e7061dd764912fc1033754.png",
            "thumbnail": {"@type": "ImageObject", "url": "https://static.loverslab.com/screenshots/monthly_2026_09/00115-4146571628.thumb.png.07e958974137c3d6ae3d3df4cb799876.png"}
        }
    ]
}
</script>
<script type='application/ld+json'>
{"@context": "http://www.schema.org", "@type": "WebSite", "name": "LoversLab", "url": "https://www.loverslab.com/"}
</script>
<script type='application/ld+json'>
{"@context": "http://schema.org", "@type": "BreadcrumbList", "itemListElement": []}
</script>
</head><body></body></html>`

func TestParseFileDetail(t *testing.T) {
	doc := parseFixture(t, fileDetailFixture)
	detail, ok := parseFileDetail(doc)
	if !ok {
		t.Fatal("expected the WebApplication block to be found")
	}

	if detail.Title != "Stable Portraits" {
		t.Errorf("Title = %q, want Stable Portraits", detail.Title)
	}
	if detail.Description == "" || len(detail.Description) < 20 {
		t.Errorf("Description = %q, want the real description text", detail.Description)
	}
	if detail.Version != "v2" {
		t.Errorf("Version = %q, want v2", detail.Version)
	}
	if detail.FileSize != "111.41 MB" {
		t.Errorf("FileSize = %q, want 111.41 MB", detail.FileSize)
	}
	if detail.Author.Name != "Karsus" || detail.Author.URL != "https://www.loverslab.com/profile/993535-karsus/" {
		t.Errorf("Author = %+v, want Karsus", detail.Author)
	}
	if detail.Views != 33402 {
		t.Errorf("Views = %d, want 33402", detail.Views)
	}
	if detail.Downloads != 6049 {
		t.Errorf("Downloads = %d, want 6049", detail.Downloads)
	}
	if detail.DateModified != "2026-09-13T20:01:52+0200" {
		t.Errorf("DateModified = %q, want the real ISO 8601 timestamp", detail.DateModified)
	}
	if len(detail.Screenshots) != 2 {
		t.Fatalf("got %d screenshots, want 2", len(detail.Screenshots))
	}
	if detail.Screenshots[0].URL == "" || detail.Screenshots[0].URL == detail.Screenshots[0].ThumbnailURL {
		t.Errorf("Screenshots[0] = %+v, want distinct full/thumbnail URLs", detail.Screenshots[0])
	}
}

// TestParseFileDetailNeverReturnsNilSlices is a real regression test: a Go nil
// slice marshals to JSON null, not [] - the frontend crashed on exactly this
// ("TypeError: null is not an object (evaluating 'screenshots.length')") for a
// file with zero screenshots and no separate rich-text description body
// found, since both fields were left at their own zero value (nil) rather
// than a real empty slice.
func TestParseFileDetailNeverReturnsNilSlices(t *testing.T) {
	doc := parseFixture(t, `<html><head>
<script type='application/ld+json'>{"@context":"http://schema.org","@type":"WebApplication","name":"No Screenshots Mod"}</script>
</head><body></body></html>`)
	detail, ok := parseFileDetail(doc)
	if !ok {
		t.Fatal("expected the WebApplication block to be found")
	}
	if detail.Screenshots == nil {
		t.Error("Screenshots is nil, want a real empty slice (would marshal to JSON null, not [])")
	}
	if detail.DescriptionBlocks == nil {
		t.Error("DescriptionBlocks is nil, want a real empty slice (would marshal to JSON null, not [])")
	}
}

func TestParseFileDetailMissingReturnsFalse(t *testing.T) {
	doc := parseFixture(t, `<html><body>no ld+json here at all</body></html>`)
	if _, ok := parseFileDetail(doc); ok {
		t.Error("expected no WebApplication block to be found")
	}
}

func TestParseFileDetailIgnoresNonWebApplicationBlocks(t *testing.T) {
	doc := parseFixture(t, `<html><head>
<script type='application/ld+json'>{"@type": "WebSite", "name": "LoversLab"}</script>
<script type='application/ld+json'>{"@type": "Organization", "name": "LoversLab"}</script>
</head><body></body></html>`)
	if _, ok := parseFileDetail(doc); ok {
		t.Error("expected no WebApplication block among only WebSite/Organization ones")
	}
}

// TestParseFileDetailAlsoParsesTheRealDescriptionBody confirms the two parsers
// (JSON-LD for everything else, the rich-text DOM for DescriptionBlocks) work
// together against one full page - the real reason DescriptionBlocks exists at
// all: the JSON-LD description field flattens away formatting, images, and a lot
// of the source page's own empty spacer paragraphs into one plain, whitespace-
// heavy string, confirmed live against the real "Lustful Void" page.
func TestParseFileDetailAlsoParsesTheRealDescriptionBody(t *testing.T) {
	doc := parseFixture(t, `<html><head>
<script type='application/ld+json'>{"@context":"http://schema.org","@type":"WebApplication","name":"Fixture Mod"}</script>
</head><body>
<div class="ipsType_richText" data-controller="core.front.core.lightboxedImages">
<p style="text-align:center;"> </p>
<p><strong>Fixture Mod</strong> is a test fixture.</p>
<p>&nbsp;</p>
<p><img src="https://static.loverslab.com/uploads/example.png" /></p>
</div>
</body></html>`)
	detail, ok := parseFileDetail(doc)
	if !ok {
		t.Fatal("expected the WebApplication block to be found")
	}
	if len(detail.DescriptionBlocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (the spacer paragraphs dropped): %+v", len(detail.DescriptionBlocks), detail.DescriptionBlocks)
	}
	if detail.DescriptionBlocks[0].ImageURL != "" {
		t.Errorf("blocks[0] = %+v, want the text block first", detail.DescriptionBlocks[0])
	}
	if detail.DescriptionBlocks[1].ImageURL != "https://static.loverslab.com/uploads/example.png" {
		t.Errorf("blocks[1] = %+v, want the image block", detail.DescriptionBlocks[1])
	}
}

func TestParseDescriptionBlocksDropsEmptySpacerParagraphs(t *testing.T) {
	doc := parseFixture(t, `<div><p> </p><p>&nbsp;</p><p>Real text.</p><p> &nbsp; </p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (three spacer paragraphs dropped): %+v", len(blocks), blocks)
	}
	if got := runsText(blocks[0].Runs); got != "Real text." {
		t.Errorf("blocks[0] text = %q, want %q", got, "Real text.")
	}
}

func TestParseDescriptionBlocksExtractsFormattingAndLinks(t *testing.T) {
	doc := parseFixture(t, `<div><p><strong>Bold</strong> and <em>italic</em> and <u>underline</u> and
<a href="https://example.com/mod">a link</a>.</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1: %+v", len(blocks), blocks)
	}
	runs := blocks[0].Runs

	find := func(text string) DescriptionRun {
		for _, r := range runs {
			if strings.Contains(r.Text, text) {
				return r
			}
		}
		t.Fatalf("no run containing %q among %+v", text, runs)
		return DescriptionRun{}
	}
	if !find("Bold").Bold {
		t.Error("the 'Bold' run should carry Bold")
	}
	if !find("italic").Italic {
		t.Error("the 'italic' run should carry Italic")
	}
	if !find("underline").Underline {
		t.Error("the 'underline' run should carry Underline")
	}
	if link := find("a link"); link.LinkURL != "https://example.com/mod" {
		t.Errorf("the 'a link' run's LinkURL = %q, want the real href", link.LinkURL)
	}
}

func TestParseDescriptionBlocksExtractsImagesAsTheirOwnBlock(t *testing.T) {
	doc := parseFixture(t, `<div><p>Before.</p><p><img src="https://static.loverslab.com/x.png" /></p><p>After.</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3: %+v", len(blocks), blocks)
	}
	if blocks[1].ImageURL != "https://static.loverslab.com/x.png" {
		t.Errorf("blocks[1] = %+v, want the image block in the middle", blocks[1])
	}
}

func TestParseDescriptionBlocksHandlesLineBreaks(t *testing.T) {
	doc := parseFixture(t, `<div><p>Line one.<br>Line two.</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1: %+v", len(blocks), blocks)
	}
	if got := runsText(blocks[0].Runs); got != "Line one.\nLine two." {
		t.Errorf("text = %q, want a real newline between the two lines", got)
	}
}

// runsText joins a block's runs back into plain text, for tests that only care
// about the words, not which runs carry which formatting.
func runsText(runs []DescriptionRun) string {
	var sb strings.Builder
	for _, r := range runs {
		sb.WriteString(r.Text)
	}
	return sb.String()
}

// The rest of these cover literal Markdown syntax pasted directly into the
// rich-text editor as plain text, confirmed live against a real Downloads
// file whose whole "About This File" is written this way
// (https://www.loverslab.com/files/file/49547-the-knights-of-the-brothel-
// english-translation/) rather than using the editor's own formatting
// toolbar - see DescriptionBlock's own comment.

func TestParseDescriptionBlocksRecognizesAtxHeadings(t *testing.T) {
	doc := parseFixture(t, `<div><p># Mod Translation &amp; Documentation</p><p>## Mod Description</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].Heading != 1 || runsText(blocks[0].Runs) != "Mod Translation & Documentation" {
		t.Errorf("blocks[0] = %+v, want Heading 1 with the '#' stripped", blocks[0])
	}
	if blocks[1].Heading != 2 || runsText(blocks[1].Runs) != "Mod Description" {
		t.Errorf("blocks[1] = %+v, want Heading 2 with the '##' stripped", blocks[1])
	}
}

func TestParseDescriptionBlocksRecognizesABlockquote(t *testing.T) {
	doc := parseFixture(t, `<div><p>&gt; <strong>Warning:</strong> read this first.</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1: %+v", len(blocks), blocks)
	}
	if !blocks[0].Quote {
		t.Errorf("blocks[0] = %+v, want Quote true", blocks[0])
	}
	if got := runsText(blocks[0].Runs); got != "Warning: read this first." {
		t.Errorf("text = %q, want the '> ' stripped", got)
	}
}

func TestParseDescriptionBlocksRecognizesAPlainTextListMarker(t *testing.T) {
	doc := parseFixture(t, `<div><p>* First point.</p><p>- Second point.</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(blocks), blocks)
	}
	if !blocks[0].ListItem || runsText(blocks[0].Runs) != "First point." {
		t.Errorf("blocks[0] = %+v, want ListItem true with the '* ' stripped", blocks[0])
	}
	if !blocks[1].ListItem || runsText(blocks[1].Runs) != "Second point." {
		t.Errorf("blocks[1] = %+v, want ListItem true with the '- ' stripped", blocks[1])
	}
}

func TestParseDescriptionBlocksMarksARealListItemToo(t *testing.T) {
	doc := parseFixture(t, `<div><ul><li>Real HTML list item.</li></ul></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 || !blocks[0].ListItem {
		t.Fatalf("got %+v, want exactly one ListItem block", blocks)
	}
}

func TestParseDescriptionBlocksRecognizesADivider(t *testing.T) {
	doc := parseFixture(t, `<div><p>Before.</p><p>---</p><p>After.</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3: %+v", len(blocks), blocks)
	}
	if !blocks[1].Divider || len(blocks[1].Runs) != 0 {
		t.Errorf("blocks[1] = %+v, want a plain Divider block with no text", blocks[1])
	}
}

func TestParseDescriptionBlocksSplitsInlineBoldMarkdown(t *testing.T) {
	doc := parseFixture(t, `<div><p>**The Grand Master:** leads the order.</p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1: %+v", len(blocks), blocks)
	}
	runs := blocks[0].Runs
	if len(runs) < 2 {
		t.Fatalf("got %d runs, want at least 2 (the bold span split from the rest): %+v", len(runs), runs)
	}
	if runs[0].Text != "The Grand Master:" || !runs[0].Bold {
		t.Errorf("runs[0] = %+v, want the bold span with the ** markers stripped", runs[0])
	}
	if got := runsText(runs); got != "The Grand Master: leads the order." {
		t.Errorf("full text = %q, want the ** markers gone from the joined text too", got)
	}
}

// A real forum "Quote" of another post - confirmed live on a real topic
// reply (a Lustful Void support-topic post quoting an earlier one):
// <blockquote class="ipsQuote" data-ipsquote-username="...">
//   <div class="ipsQuote_citation">2 hours ago, X said:</div>
//   <div class="ipsQuote_contents"><p>...</p></div>
// </blockquote>
// followed by the replying member's own new text.
func TestParseDescriptionBlocksExtractsARealForumQuote(t *testing.T) {
	doc := parseFixture(t, `<div>
<blockquote class="ipsQuote" data-ipsquote="" data-ipsquote-username="darkspleen" data-ipsquote-contentcommentid="2575105">
	<div class="ipsQuote_citation">2 hours ago, darkspleen said:</div>
	<div class="ipsQuote_contents"><p>We have arrived at the promised land.</p></div>
</blockquote>
<p>masha allah, comrades</p>
</div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (the quote, then the reply's own text): %+v", len(blocks), blocks)
	}
	quote := blocks[0]
	if quote.QuotedAuthor != "darkspleen" {
		t.Errorf("QuotedAuthor = %q, want darkspleen", quote.QuotedAuthor)
	}
	if len(quote.QuotedBlocks) != 1 || runsText(quote.QuotedBlocks[0].Runs) != "We have arrived at the promised land." {
		t.Errorf("QuotedBlocks = %+v, want the quoted excerpt's own real text", quote.QuotedBlocks)
	}
	// The citation's own "2 hours ago, darkspleen said:" text is never
	// surfaced as a run anywhere - QuotedAuthor is the real, reusable signal
	// instead of that page-relative, unreformattable phrasing.
	if runsText(blocks[1].Runs) != "masha allah, comrades" {
		t.Errorf("blocks[1] = %+v, want the reply's own new text, unaffected by the quote above it", blocks[1])
	}
}

// A real inline emoticon (confirmed live: a real reply's own
// "<img data-emoticon="" src=".../smiley.png" alt=":D" title=":D"/>") must
// stay inline with the sentence around it, at icon size - not become its own
// full description-image block, which is what made an emoji render
// "massive" (full description-image width) before this.
func TestParseDescriptionBlocksKeepsARealEmoticonInline(t *testing.T) {
	doc := parseFixture(t, `<div><p>It is here! I can finally enjoy Stellaris again <img alt=":D" data-emoticon="" src="https://www.loverslab.com/resources/emoticons/smiley.png" title=":D"/></p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (the emoticon stays inline, not its own block): %+v", len(blocks), blocks)
	}
	runs := blocks[0].Runs
	var found bool
	for _, r := range runs {
		if r.EmoteURL == "https://www.loverslab.com/resources/emoticons/smiley.png" {
			found = true
			if r.EmoteAlt != ":D" {
				t.Errorf("EmoteAlt = %q, want :D", r.EmoteAlt)
			}
		}
	}
	if !found {
		t.Errorf("no run carried the real emoticon URL: %+v", runs)
	}
	if runsText(runs) != "It is here! I can finally enjoy Stellaris again " {
		t.Errorf("surrounding text = %q, want the sentence kept intact around the emoticon", runsText(runs))
	}
}

// A message that is *only* an emoticon (no other text at all) must survive
// trimRuns - an empty-Text run is otherwise indistinguishable from a
// whitespace-only spacer run, which trimRuns exists specifically to drop.
func TestParseDescriptionBlocksKeepsAMessageThatIsOnlyAnEmoticon(t *testing.T) {
	doc := parseFixture(t, `<div><p><img data-emoticon="" src="https://www.loverslab.com/resources/emoticons/smiley.png" alt=":D"/></p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 || len(blocks[0].Runs) != 1 || blocks[0].Runs[0].EmoteURL == "" {
		t.Fatalf("got %+v, want exactly one block with the emoticon run kept", blocks)
	}
}

// A real attached image (confirmed live:
// <a class="ipsAttachLink ipsAttachLink_image"><img ...></a>, inside a forum
// post) must render inline like any other embedded image - it was
// previously dropped entirely (the whole ipsAttachLink subtree skipped, on
// the assumption the separate attachment chip already covered it, which
// only ever showed the filename, never the actual picture).
func TestParsePostContentBlocksShowsARealAttachedImageInline(t *testing.T) {
	doc := parseFixture(t, `<div>
<p>So about that first release...</p>
<p><a class="ipsAttachLink ipsAttachLink_image" href="https://www.loverslab.com/uploads/monthly_2019_04/done.png"><img data-fileid="668532" src="https://www.loverslab.com/uploads/monthly_2019_04/done.png" width="490" class="ipsImage ipsImage_thumbnailed" alt="done.png"/></a></p>
</div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parsePostContentBlocks(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (the text, then the real image): %+v", len(blocks), blocks)
	}
	if blocks[1].ImageURL != "https://www.loverslab.com/uploads/monthly_2019_04/done.png" {
		t.Errorf("blocks[1] = %+v, want the attached image's own real URL", blocks[1])
	}
}

// A non-image attachment (a .zip, a log file) still has no sensible inline
// rendering - only its own separate attachment chip covers it, same as
// before.
func TestParsePostContentBlocksStillDropsANonImageAttachmentLink(t *testing.T) {
	doc := parseFixture(t, `<div><p>Here's the beta build. <a class="ipsAttachLink" href="https://www.loverslab.com/uploads/x.zip">build.zip</a></p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parsePostContentBlocks(body)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1: %+v", len(blocks), blocks)
	}
	if runsText(blocks[0].Runs) != "Here's the beta build." {
		t.Errorf("text = %q, want the attachment link's own label excluded", runsText(blocks[0].Runs))
	}
}

// A real embedded reference to another LoversLab topic/comment (confirmed
// live: <iframe data-controller="core.front.core.autosizeiframe"
// data-embedid="...">, IPS's own rich preview when a member pastes a link to
// another post) becomes a real, clickable EmbedURL block rather than being
// silently dropped or rendered as a dead iframe this app's webview can't
// usefully load.
func TestParseDescriptionBlocksExtractsARealEmbeddedTopicReference(t *testing.T) {
	doc := parseFixture(t, `<div><p>Is this compatible?</p>
<iframe allowfullscreen="" data-controller="core.front.core.autosizeiframe" data-embedid="embed2280254930" src="https://www.loverslab.com/topic/91443-example/?do=embed&amp;comment=2558391"></iframe>
</div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2 (the text, then the embed): %+v", len(blocks), blocks)
	}
	if blocks[1].EmbedURL != "https://www.loverslab.com/topic/91443-example/?do=embed&comment=2558391" {
		t.Errorf("blocks[1].EmbedURL = %q, want the real embedded iframe's own src", blocks[1].EmbedURL)
	}
}

func TestParseDescriptionBlocksNeverTreatsARealLinksLabelAsMarkdownBold(t *testing.T) {
	doc := parseFixture(t, `<div><p><a href="https://example.com/**weird**">a link</a></p></div>`)
	body := findOne(doc, func(n *html.Node) bool { return isElement(n, "div") })
	blocks := parseDescriptionBlocks(body)
	if len(blocks) != 1 || len(blocks[0].Runs) != 1 {
		t.Fatalf("got %+v, want exactly one run", blocks)
	}
	if blocks[0].Runs[0].Text != "a link" || blocks[0].Runs[0].Bold {
		t.Errorf("runs[0] = %+v, want the link's own label left untouched", blocks[0].Runs[0])
	}
}
