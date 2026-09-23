package loverslab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// storedCookie is the subset of http.Cookie worth persisting between runs.
type storedCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ExportSession returns the current cookie jar as a JSON string, for the
// caller to persist however it sees fit - see loverslab.go, which seals this
// through internal/credentials rather than writing it to a plain file the
// way the original research prototype did, since it is just as much a live
// credential as the username and password already saved there.
func (c *Client) ExportSession() (string, error) {
	u, err := url.Parse(BaseURL)
	if err != nil {
		return "", err
	}

	var stored []storedCookie
	for _, ck := range c.jar.Cookies(u) {
		stored = append(stored, storedCookie{Name: ck.Name, Value: ck.Value})
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return "", fmt.Errorf("exporting session: %w", err)
	}
	return string(data), nil
}

// ImportSession restores cookies previously returned by ExportSession. It
// does not verify the session is still valid - call VerifySession (or
// IsLoggedIn for a cheaper, local-only check) afterward to confirm.
func (c *Client) ImportSession(data string) error {
	var stored []storedCookie
	if err := json.Unmarshal([]byte(data), &stored); err != nil {
		return fmt.Errorf("importing session: %w", err)
	}

	u, err := url.Parse(BaseURL)
	if err != nil {
		return err
	}

	cookies := make([]*http.Cookie, 0, len(stored))
	for _, sc := range stored {
		cookies = append(cookies, &http.Cookie{Name: sc.Name, Value: sc.Value})
	}
	c.jar.SetCookies(u, cookies)

	// A different session (a different account, or the same one signing back
	// in) can see genuinely different content (private-to-account state,
	// follow/reaction status) - never let a page cached under a previous
	// session outlive it.
	c.cacheMu.Lock()
	c.cache = nil
	c.cacheMu.Unlock()
	return nil
}
