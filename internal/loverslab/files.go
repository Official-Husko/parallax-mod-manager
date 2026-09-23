package loverslab

import (
	"context"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
)

// FileDownload is one downloadable version/attachment of a Downloads file,
// e.g. a mod's main archive or an update.
type FileDownload struct {
	Name string
	URL  string // one-time, csrfKey-guarded link good for a single download
}

// fileDownloadPattern matches each entry in the "Download your files"
// dialog IPS renders at <file page>?do=download: a version's display name,
// followed (non-greedily, so it pairs with the nearest one) by its
// download button's href.
var fileDownloadPattern = regexp.MustCompile(`(?s)<span class='ipsType_break ipsContained'>([^<]+)</span>.*?<a href='([^']+)' class='ipsButton ipsButton_primary ipsButton_small' data-action="download"`)

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
func parseDownloadDialog(body []byte) []FileDownload {
	matches := fileDownloadPattern.FindAllSubmatch(body, -1)
	if matches == nil {
		return nil
	}
	downloads := make([]FileDownload, 0, len(matches))
	for _, m := range matches {
		downloads = append(downloads, FileDownload{
			Name: html.UnescapeString(string(m[1])),
			URL:  html.UnescapeString(string(m[2])),
		})
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
