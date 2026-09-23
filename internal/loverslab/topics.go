package loverslab

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

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
	Posted          string // as displayed by the site
	URL             string // deep link straight to this post
	Content         string // the reply's own text, rendered plain (see commentText below) - attachment links are excluded, since Attachments already covers them
	Attachments     []PostAttachment
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

		var author, authorURL, authorAvatarURL string
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

		content := ""
		if body := findOne(article, func(n *html.Node) bool { return attrOr(n, "data-role") == "commentContent" }); body != nil {
			content = commentText(body)
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
			Posted:          posted,
			URL:             postURL,
			Content:         content,
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
