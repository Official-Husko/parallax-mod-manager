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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"golang.org/x/net/html"
)

const (
	// BaseURL is the site root. All requests are made against this host.
	BaseURL = "https://www.loverslab.com"

	// userAgent mirrors a real browser; Cloudflare and IPS both treat
	// default Go/http user agents with more suspicion.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
)

// Client wraps an http.Client with a persistent cookie jar, since the whole
// IPS4 session (CSRF key, login state) rides on cookies.
type Client struct {
	http *http.Client
	jar  http.CookieJar
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

// getDocument fetches pageURL and parses it as HTML for DOM-based scraping.
func (c *Client) getDocument(ctx context.Context, pageURL string) (*html.Node, error) {
	req, err := c.newRequest(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", pageURL, err)
	}
	return doc, nil
}
