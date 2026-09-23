// Package loverslab implements a minimal HTTP client for loverslab.com, an
// Invision Community (IPS4) forum, for the Browse tab's LoversLab source. It
// drives the site the same way a browser would (cookie jar, matching
// headers) - loverslab.com has no public API, so this was built by reading
// real requests/responses against a real account; see docs/loverslab.md for
// the site behavior this was reverse-engineered from and the reasoning
// behind each of the choices below.
//
// This package only knows how to talk to the site - session persistence
// (encrypting the cookie jar and the sign-in itself at rest) is loverslab.go
// and loverslab_settings.go's job, one layer up, using internal/credentials.
package loverslab

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	// BaseURL is the site root. All requests are made against this host.
	BaseURL = "https://www.loverslab.com"

	// userAgent mirrors a real browser; Cloudflare and IPS both treat
	// default Go/http user agents with more suspicion.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

	// pageCacheTTL is how long getDocument reuses a page's own already-
	// fetched bytes instead of making a fresh request for the exact same
	// URL - long enough that paging back and forth, or re-opening a mod
	// that's already been looked at, feels instant instead of visibly
	// re-fetching every single time (confirmed a real source of the
	// "browsing feels laggy" report - every one of these was a real,
	// uncached network round-trip before this existed), short enough that a
	// real change (a new reply, a newly posted file) still shows up within
	// a few minutes rather than sitting stale for a whole session.
	pageCacheTTL = 3 * time.Minute
)

// Client wraps an http.Client with a persistent cookie jar, since the whole
// IPS4 session (CSRF key, login state) rides on cookies.
type Client struct {
	http *http.Client
	jar  http.CookieJar

	cacheMu sync.Mutex
	cache   map[string]cachedPage
}

// cachedPage is one getDocument fetch's own result, cached by its exact URL.
// The raw bytes are kept rather than the already-parsed *html.Node tree, so
// every cache hit gets its own fresh, independent tree to walk - cheap
// (html.Parse on an already-fetched page is local CPU work, no network) and
// avoids any question of whether walking a shared tree could ever mutate it
// for a later caller.
type cachedPage struct {
	body    []byte
	fetched time.Time
}

// New creates a Client with a fresh, empty session.
func New() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		jar: jar,
	}, nil
}

// newRequest builds a request with the headers the site expects from a
// browser-driven session. body may be nil for GET-style requests.
func (c *Client) newRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

// getDocument fetches pageURL and parses it as HTML for DOM-based scraping -
// reusing a recent enough fetch of the exact same URL (see pageCacheTTL)
// rather than making a fresh request every time.
func (c *Client) getDocument(ctx context.Context, pageURL string) (*html.Node, error) {
	if body := c.cachedBody(pageURL); body != nil {
		doc, err := html.Parse(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", pageURL, err)
		}
		return doc, nil
	}

	req, err := c.newRequest(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", pageURL, err)
	}
	c.storeCachedBody(pageURL, body)

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", pageURL, err)
	}
	return doc, nil
}

func (c *Client) cachedBody(url string) []byte {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	entry, ok := c.cache[url]
	if !ok || time.Since(entry.fetched) > pageCacheTTL {
		return nil
	}
	return entry.body
}

func (c *Client) storeCachedBody(url string, body []byte) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.cache == nil {
		c.cache = map[string]cachedPage{}
	}
	c.cache[url] = cachedPage{body: body, fetched: time.Now()}
}

// invalidateCachePrefix drops every cached page whose URL starts with
// prefix - used after writing something (posting a comment) so the very
// next read reliably sees it, rather than a stale cached copy of whichever
// page(s) that write actually changed. A prefix, not one exact URL: a
// forum topic's own later pages (".../page/2/", ...) are each their own
// separate cache entry, and posting a reply can land on any of them.
func (c *Client) invalidateCachePrefix(prefix string) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	for key := range c.cache {
		if strings.HasPrefix(key, prefix) {
			delete(c.cache, key)
		}
	}
}
