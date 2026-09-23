package loverslab

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// postCountTitlePattern matches the author panel's own "<a title="215
// posts">" (or the singular "1 post", never confirmed live but a reasonable
// site-wide English pluralization to expect) - the real, site-wide post
// count on that member's own profile, not this topic's own reply count.
var postCountTitlePattern = regexp.MustCompile(`^(\d+)\s+posts?$`)

// PostAttachment is a file a user attached directly to a forum post/reply - e.g. a
// crash log, an unofficial patch, or a screenshot shared in a mod's support topic.
// This is distinct from the mod's own official downloads (see ListDownloads); it's
// whatever other members have shared in the discussion around it.
type PostAttachment struct {
	Filename  string
	Extension string // from the site's own data-fileext; usually empty for inline images
	URL       string
	IsImage   bool
}

// Post is one reply in a forum topic (most usefully, a Downloads file's linked "Get
// Support" topic).
type Post struct {
	ID        string
	Author    string
	AuthorURL string
	// AuthorAvatarURL is the author's own profile photo, from the same author-info
	// panel as Author/AuthorURL - "" for a member with no avatar set, same as
	// FileAuthor.ImageURL. Publicly hosted, same CDN as every other image this
	// package reads (no auth needed to load it).
	AuthorAvatarURL string
	// AuthorGroup is the author's own membership status as the site displays it
	// (e.g. "Members", "Advanced Member") - from the author panel's own
	// data-role="group", confirmed live on a real topic.
	AuthorGroup string
	// AuthorPostCount is the author's own real, site-wide post count (that
	// profile link's own title attribute, "N posts") - not this topic's own
	// reply count, the same number shown on their profile.
	AuthorPostCount int
	// AuthorTitle is a custom tagline the member set on their own profile
	// (data-role="custom-field"), if they set one at all - "" otherwise, never
	// a placeholder.
	AuthorTitle string
	// IsTopicAuthor is true when this reply's own author is whoever started
	// this topic (the real strong.ipsComment_authorBadge "Author" badge,
	// confirmed live) - true for every reply they post in it, not just their
	// very first one.
	IsTopicAuthor bool
	// IsPopular is true for a reply the site itself flagged "Popular Post"
	// (strong.ipsBadge_popular) - its own real reaction-threshold badge, never
	// something this app decides.
	IsPopular bool
	// Reactions is the real reaction count on this reply (data-role=
	// "reactCountText") - 0 for a reply with none, not the absence of a field.
	Reactions int
	// Edited is true when the site itself shows an "(edited)" note next to
	// this reply's posted date.
	Edited          bool
	Posted          string // as displayed by the site
	URL             string // deep link straight to this post
	Content         string // the reply's own text, rendered plain (see commentText below) - attachment links are excluded, since Attachments already covers them
	// ContentBlocks is the same reply, parsed the same real way a Downloads
	// file's own description and changelog are (see
	// internal/loverslab/detail.go's parseDescriptionBlocks): real formatting,
	// embedded images, and a real forum "Quote" of another post rendered as
	// its own attributed block rather than flattened into the surrounding
	// text. Never nil, same as every other DescriptionBlock slice this
	// package returns.
	ContentBlocks []DescriptionBlock
	Attachments   []PostAttachment
}

// ListTopicPosts returns the posts on one (1-indexed) page of a forum topic, plus the
// total page count. Each post's Attachments field surfaces any files users attached
// directly to their replies.
func (c *Client) ListTopicPosts(ctx context.Context, topicURL string, page int) ([]Post, int, error) {
	pageURL := topicURL
	if page > 1 {
		pageURL = strings.TrimRight(topicURL, "/") + fmt.Sprintf("/page/%d/", page)
	}

	doc, err := c.getDocument(ctx, pageURL)
	if err != nil {
		return nil, 0, fmt.Errorf("listing topic posts: %w", err)
	}
	posts, totalPages := parseTopicPosts(doc)
	return posts, totalPages, nil
}

// parseTopicPosts is ListTopicPosts' own parsing, pulled out so it can be tested
// directly against a hand-built document instead of a real request - see
// topics_test.go.
func parseTopicPosts(doc *html.Node) ([]Post, int) {
	totalPages := 1
	if pag := findOne(doc, func(n *html.Node) bool { return isElement(n, "ul") && hasClass(n, "ipsPagination") }); pag != nil {
		if v := attrOr(pag, "data-pages"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				totalPages = n
			}
		}
	}

	articles := find(doc, func(n *html.Node) bool { return isElement(n, "article") && hasClass(n, "ipsComment") })

	posts := make([]Post, 0, len(articles))
	for _, article := range articles {
		id := strings.TrimPrefix(attrOr(article, "id"), "elComment_")

		var author, authorURL, authorAvatarURL, authorGroup, authorTitle string
		var authorPostCount int
		if aside := findOne(article, func(n *html.Node) bool { return isElement(n, "aside") && hasClass(n, "cAuthorPane") }); aside != nil {
			if a := findOne(aside, func(n *html.Node) bool { return isElement(n, "a") && strings.Contains(attrOr(n, "href"), "/profile/") }); a != nil {
				authorURL = attrOr(a, "href")
				author = strings.TrimSpace(text(a))
			}
			// The desktop author panel's own photo - scoped to this aside
			// specifically (not the article as a whole) so the separate,
			// mobile-only author panel earlier in the same article never wins
			// instead.
			if img := findOne(aside, func(n *html.Node) bool { return isElement(n, "img") }); img != nil {
				authorAvatarURL = attrOr(img, "src")
			}
			// The author's own real membership status (e.g. "Members"), post
			// count (that same panel's own "<a title="215 posts">"), and
			// custom tagline, if they set one - all confirmed live against a
			// real topic's own author panel.
			if g := findOne(aside, func(n *html.Node) bool { return attrOr(n, "data-role") == "group" }); g != nil {
				authorGroup = strings.TrimSpace(text(g))
			}
			if pc := findOne(aside, func(n *html.Node) bool {
				return isElement(n, "a") && postCountTitlePattern.MatchString(attrOr(n, "title"))
			}); pc != nil {
				if m := postCountTitlePattern.FindStringSubmatch(attrOr(pc, "title")); m != nil {
					authorPostCount, _ = strconv.Atoi(m[1])
				}
			}
			if cf := findOne(aside, func(n *html.Node) bool { return attrOr(n, "data-role") == "custom-field" }); cf != nil {
				authorTitle = strings.TrimSpace(text(cf))
			}
		}

		posted := ""
		if t := findOne(article, func(n *html.Node) bool { return isElement(n, "time") }); t != nil {
			posted = strings.TrimSpace(text(t))
		}

		postURL := ""
		if link := findOne(article, func(n *html.Node) bool {
			return isElement(n, "a") && strings.Contains(attrOr(n, "href"), "#findComment-")
		}); link != nil {
			postURL = attrOr(link, "href")
		}

		// This reply's own real "Author" badge (this topic's own starter,
		// confirmed live to appear on every reply they post in it, not just
		// their very first) and "Popular Post" badge - both the site's own,
		// never guessed. Each is duplicated once for the mobile layout and
		// once for desktop in the real markup; either match is enough.
		isTopicAuthor := findOne(article, func(n *html.Node) bool { return hasClass(n, "ipsComment_authorBadge") }) != nil
		isPopular := findOne(article, func(n *html.Node) bool { return hasClass(n, "ipsBadge_popular") }) != nil

		edited := findOne(article, func(n *html.Node) bool {
			return isElement(n, "span") && strings.TrimSpace(text(n)) == "(edited)"
		}) != nil

		reactions := 0
		if rc := findOne(article, func(n *html.Node) bool { return attrOr(n, "data-role") == "reactCountText" }); rc != nil {
			reactions, _ = strconv.Atoi(strings.TrimSpace(text(rc)))
		}

		content := ""
		contentBlocks := []DescriptionBlock{}
		if body := findOne(article, func(n *html.Node) bool { return attrOr(n, "data-role") == "commentContent" }); body != nil {
			content = commentText(body)
			contentBlocks = parsePostContentBlocks(body)
		}

		attachmentLinks := find(article, func(n *html.Node) bool { return isElement(n, "a") && hasClass(n, "ipsAttachLink") })
		attachments := make([]PostAttachment, 0, len(attachmentLinks))
		for _, a := range attachmentLinks {
			isImage := hasClass(a, "ipsAttachLink_image")
			filename := strings.TrimSpace(text(a))
			// An image attachment wraps a bare <img> with no text of its own - the
			// original filename only survives in its alt text.
			if isImage && filename == "" {
				if img := findOne(a, func(n *html.Node) bool { return isElement(n, "img") }); img != nil {
					filename = attrOr(img, "alt")
				}
			}
			attachments = append(attachments, PostAttachment{
				Filename:  filename,
				Extension: attrOr(a, "data-fileext"),
				URL:       attrOr(a, "href"),
				IsImage:   isImage,
			})
		}

		posts = append(posts, Post{
			ID:              id,
			Author:          author,
			AuthorURL:       authorURL,
			AuthorAvatarURL: authorAvatarURL,
			AuthorGroup:     authorGroup,
			AuthorPostCount: authorPostCount,
			AuthorTitle:     authorTitle,
			IsTopicAuthor:   isTopicAuthor,
			IsPopular:       isPopular,
			Reactions:       reactions,
			Edited:          edited,
			Posted:          posted,
			URL:             postURL,
			Content:         content,
			ContentBlocks:   contentBlocks,
			Attachments:     attachments,
		})
	}
	return posts, totalPages
}

// commentText renders a reply's content the same way richText (htmlutil.go) does -
// paragraphs, <br> line breaks, ordinary links as "label (href)" - except an
// attachment link is skipped entirely rather than rendered inline: parseTopicPosts
// already surfaces every attachment separately as its own PostAttachment, so
// including it a second time, as a raw upload URL trailing the sentence, would just
// be redundant with the attachment chip the frontend already shows for it.
func commentText(n *html.Node) string {
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
		case isElement(n, "a") && hasClass(n, "ipsAttachLink"):
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

	lines := strings.Split(sb.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// ListFileSupportPosts resolves a Downloads file's linked "Get Support" topic and
// returns its posts, as ListTopicPosts. If the file has no support topic linked at
// all, it returns (nil, 0, nil) - not an error, since plenty of files simply don't
// have one.
func (c *Client) ListFileSupportPosts(ctx context.Context, filePageURL string, page int) ([]Post, int, error) {
	topicURL, err := c.SupportTopicURL(ctx, filePageURL)
	if err != nil {
		return nil, 0, err
	}
	if topicURL == "" {
		return nil, 0, nil
	}
	return c.ListTopicPosts(ctx, topicURL, page)
}
