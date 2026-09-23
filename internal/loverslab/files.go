package loverslab

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// FileDownload is one downloadable version/attachment of a Downloads file,
// e.g. a mod's main archive or an update.
type FileDownload struct {
	Name string
	URL  string // one-time, csrfKey-guarded link good for a single download

	// Size and Posted are exactly as the download dialog itself displays
	// them (e.g. "2.43 MB", "December 8, 2020") - display strings, not a
	// parsed byte count or timestamp, the same as this package's other
	// display-only date fields (FileSummary.Updated, ChangelogEntry.Released).
	// Either can be empty if the dialog's markup didn't have it.
	Size   string
	Posted string
}

// ListDownloads fetches the download dialog for a Downloads file page
// (e.g. https://www.loverslab.com/files/file/50753-tether-hand-holding-for-followers/)
// and returns each attached version with its one-time download URL.
func (c *Client) ListDownloads(ctx context.Context, filePageURL string) ([]FileDownload, error) {
	req, err := c.newRequest(ctx, http.MethodGet, filePageURL+"?do=download", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", filePageURL)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing downloads: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("listing downloads: %w", err)
	}

	downloads := parseDownloadDialog(body)
	if downloads == nil {
		return nil, fmt.Errorf("no downloadable files found on %s (not logged in, no download permission, or page layout changed)", filePageURL)
	}
	return downloads, nil
}

// parseDownloadDialog is ListDownloads' own parsing, pulled out so it can be
// tested directly against a fixture instead of a real request - see
// files_test.go.
//
// Each version in the dialog is one <li class='ipsDataItem'>, confirmed live
// against the real, authenticated dialog:
//
//	<li class='ipsDataItem'>
//	  <div class='ipsDataItem_main'>
//	    <h4 class='ipsDataItem_title ...'><span class='ipsType_break ipsContained'>LV Lewd Rooms.zip</span></h4>
//	    <p class='ipsType_reset ipsDataItem_meta'>
//	      2.43 MB
//	      <span class='ipsType_neutral'> / <time datetime='...' title='...'>December 8, 2020</time></span>
//	    </p>
//	  </div>
//	  <div class='ipsDataItem_generic ...'>
//	    <a href='...' class='ipsButton ipsButton_primary ipsButton_small' data-action="download">Download</a>
//	  </div>
//	</li>
//
// DOM-based (matching htmlutil.go's helpers) rather than regex, since a
// version's own name/size/date can contain arbitrary text a regex would
// mis-split on.
func parseDownloadDialog(body []byte) []FileDownload {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	items := find(doc, func(n *html.Node) bool {
		return isElement(n, "li") && hasClass(n, "ipsDataItem")
	})

	downloads := make([]FileDownload, 0, len(items))
	for _, item := range items {
		nameNode := findOne(item, func(n *html.Node) bool {
			return isElement(n, "span") && hasClass(n, "ipsType_break") && hasClass(n, "ipsContained")
		})
		link := findOne(item, func(n *html.Node) bool {
			return isElement(n, "a") && attrOr(n, "data-action") == "download"
		})
		if nameNode == nil || link == nil {
			continue
		}

		d := FileDownload{
			Name: strings.TrimSpace(text(nameNode)),
			URL:  attrOr(link, "href"),
		}

		if meta := findOne(item, func(n *html.Node) bool {
			return isElement(n, "p") && hasClass(n, "ipsDataItem_meta")
		}); meta != nil {
			// The size is the meta paragraph's own leading text, i.e.
			// everything before its first child element (the " / <time>"
			// suffix) - never that suffix's own text.
			var size strings.Builder
			for c := meta.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.TextNode {
					break
				}
				size.WriteString(c.Data)
			}
			d.Size = collapseWhitespace(strings.TrimSpace(size.String()))

			if t := findOne(meta, func(n *html.Node) bool { return isElement(n, "time") }); t != nil {
				d.Posted = strings.TrimSpace(text(t))
			}
		}

		downloads = append(downloads, d)
	}
	if len(downloads) == 0 {
		return nil
	}
	return downloads
}

// DownloadFile streams a file version (as returned by ListDownloads) to w, and returns
// the filename the server suggests via Content-Disposition. onProgress, if not nil, is
// called periodically as the body streams in - done is bytes copied so far, total is
// the response's own Content-Length, or -1 when the server didn't send one (a caller
// wanting a determinate progress bar should treat that as "unknown", not zero).
func (c *Client) DownloadFile(ctx context.Context, downloadURL string, w io.Writer, onProgress func(done, total int64)) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading file: unexpected status %s", resp.Status)
	}

	filename := filenameFromContentDisposition(resp.Header.Get("Content-Disposition"))

	dst := w
	if onProgress != nil {
		total := resp.ContentLength // -1 when absent, matching this method's own documented meaning
		dst = &progressWriter{w: w, total: total, onProgress: onProgress}
	}
	if _, err := io.Copy(dst, resp.Body); err != nil {
		return filename, fmt.Errorf("downloading file: %w", err)
	}
	return filename, nil
}

// progressWriter wraps an io.Writer, reporting cumulative bytes written after every
// Write call - DownloadFile's own progress reporting, kept here rather than inline so
// io.Copy can use it directly without DownloadFile re-implementing the copy loop.
type progressWriter struct {
	w          io.Writer
	done       int64
	total      int64
	onProgress func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	p.onProgress(p.done, p.total)
	return n, err
}

func filenameFromContentDisposition(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	name := params["filename"]
	if decoded, err := url.QueryUnescape(name); err == nil {
		return decoded
	}
	return name
}
