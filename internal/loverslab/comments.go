package loverslab

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
)

var topicIDPattern = regexp.MustCompile(`/topic/(\d+)-`)

// PostComment submits a reply to a forum topic - most usefully, a Downloads file's
// linked "Get Support" topic (see SupportTopicURL). content is simple HTML (e.g.
// "<p>...</p>"), matching what the site's own rich text editor submits - see
// docs/loverslab.md's Notifications/Comments sections for how this was confirmed.
//
// The underlying form is multipart (it doubles as a file-attachment upload), so this
// builds one even though no file is attached here.
func (c *Client) PostComment(ctx context.Context, topicURL, content string) error {
	m := topicIDPattern.FindStringSubmatch(topicURL)
	if m == nil {
		return fmt.Errorf("posting comment: %q doesn't look like a topic URL", topicURL)
	}
	topicID := m[1]

	key, err := c.csrfKey(ctx, topicURL)
	if err != nil {
		return fmt.Errorf("posting comment: %w", err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fields := map[string]string{
		"commentform_" + topicID + "_submitted": "1",
		"csrfKey":                               key,
		"_contentReply":                         "1",
		"topic_comment_" + topicID:              content,
		"topic_auto_follow":                     "0",
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return fmt.Errorf("posting comment: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("posting comment: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, topicURL, &body)
	if err != nil {
		return fmt.Errorf("posting comment: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", topicURL)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("posting comment: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("posting comment: unexpected status %s", resp.Status)
	}

	// The topic's own pages (getDocument's cache is keyed by exact URL, and a
	// reply can land on any of them) must not still serve a stale, pre-reply
	// copy to the very next read - see invalidateCachePrefix.
	c.invalidateCachePrefix(topicURL)
	return nil
}
