package loverslab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const loginPath = "/login/"

// csrfKeyPattern matches IPS4's hidden csrfKey field, e.g.
// <input type="hidden" name="csrfKey" value="4ca509366a475a48b16d72038234199b">
var csrfKeyPattern = regexp.MustCompile(`name=['"]csrfKey['"]\s+value=['"]([a-fA-F0-9]+)['"]`)

// csrfKey fetches a page and pulls the current csrfKey out of it. IPS
// rotates this per session/page, so it must be read fresh before each
// state-changing POST (login, logout, ...).
func (c *Client) csrfKey(ctx context.Context, pageURL string) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", pageURL, err)
	}

	key, ok := extractCSRFKey(body)
	if !ok {
		return "", fmt.Errorf("csrfKey not found on %s", pageURL)
	}
	return key, nil
}

// extractCSRFKey is csrfKey's own parsing, pulled out so it can be tested
// directly against a fixture instead of a real request - see auth_test.go.
func extractCSRFKey(body []byte) (string, bool) {
	match := csrfKeyPattern.FindSubmatch(body)
	if match == nil {
		return "", false
	}
	return string(match[1]), true
}

// Login authenticates with an email/username and password, replicating the
// site's own /login/ form submission (auth + password + csrfKey).
func (c *Client) Login(ctx context.Context, auth, password string) error {
	key, err := c.csrfKey(ctx, BaseURL+loginPath)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	form := url.Values{
		"csrfKey":       {key},
		"auth":          {auth},
		"password":      {password},
		"remember_me":   {"1"},
		"_processLogin": {"usernamepassword"},
	}

	req, err := c.newRequest(ctx, http.MethodPost, BaseURL+loginPath, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", BaseURL+loginPath)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("login: submitting form: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if !c.IsLoggedIn() {
		return errors.New("login: rejected (bad credentials, captcha, or 2FA challenge)")
	}
	return nil
}

// Logout ends the current session, mirroring the site's logout link
// (which itself is just a csrfKey-guarded GET).
func (c *Client) Logout(ctx context.Context) error {
	key, err := c.csrfKey(ctx, BaseURL+"/")
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}

	logoutURL := fmt.Sprintf("%s/logout/?csrfKey=%s", BaseURL, key)
	req, err := c.newRequest(ctx, http.MethodGet, logoutURL, nil)
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// IsLoggedIn reports whether the session currently holds an authenticated
// member cookie. It's a local check against the cookie jar, not a network call.
func (c *Client) IsLoggedIn() bool {
	return c.cookie("ips4_member_id") != ""
}

// VerifySession asks the site itself whether the current session is really
// still valid, via the x-ips-loggedin response header IPS sends on every
// request (confirmed "1" when logged in, "0" otherwise) - unlike IsLoggedIn,
// this catches a session that looks complete locally (all three remember-me
// cookies present) but was actually revoked server-side (a changed
// password, a manual "log out everywhere"), since a restored session from a
// previous run is only ever a guess about its own validity until asked.
func (c *Client) VerifySession(ctx context.Context) (bool, error) {
	if !c.IsLoggedIn() {
		return false, nil
	}
	req, err := c.newRequest(ctx, http.MethodGet, BaseURL+"/", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("verifying session: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.Header.Get("x-ips-loggedin") == "1", nil
}

// MemberID returns the logged-in member's numeric ID, or "" if not logged in.
func (c *Client) MemberID() string {
	return c.cookie("ips4_member_id")
}

func (c *Client) cookie(name string) string {
	u, err := url.Parse(BaseURL)
	if err != nil {
		return ""
	}
	for _, ck := range c.jar.Cookies(u) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

// AccountProfile is the signed-in account's own real display name and profile
// link, from the page header's own account menu - distinct from whatever was
// typed to sign in (an email works just as well as a username for that, and is
// never the same string as the site's own display name). Confirmed live against
// a real account: the display name shown, the "Profile" menu item's own href,
// and the small header avatar (a data: URI monogram for an account with no
// uploaded photo, same as any other <img src> as far as rendering it goes).
type AccountProfile struct {
	Username   string
	ProfileURL string
	AvatarURL  string
}

// AccountProfile fetches the signed-in account's own display name, profile
// link, and avatar - "", false if the session isn't actually signed in (the
// header simply has no account menu to find, then).
func (c *Client) AccountProfile(ctx context.Context) (AccountProfile, bool, error) {
	doc, err := c.getDocument(ctx, BaseURL+"/")
	if err != nil {
		return AccountProfile{}, false, fmt.Errorf("finding the signed-in account's own profile: %w", err)
	}
	return parseAccountProfile(doc)
}

// parseAccountProfile is AccountProfile's own parsing, pulled out so it can be
// tested directly against a hand-built fragment instead of a real request -
// see auth_test.go.
func parseAccountProfile(doc *html.Node) (AccountProfile, bool, error) {
	userLi := findOne(doc, func(n *html.Node) bool { return isElement(n, "li") && attrOr(n, "id") == "cUserLink" })
	if userLi == nil {
		return AccountProfile{}, false, nil
	}

	nameLink := findOne(userLi, func(n *html.Node) bool { return isElement(n, "a") && attrOr(n, "id") == "elUserLink" })
	if nameLink == nil {
		return AccountProfile{}, false, nil
	}
	username := strings.TrimSpace(text(nameLink))

	var profileURL, avatarURL string
	if photoLink := findOne(userLi, func(n *html.Node) bool {
		return isElement(n, "a") && strings.Contains(attrOr(n, "href"), "/profile/")
	}); photoLink != nil {
		profileURL = attrOr(photoLink, "href")
		if img := findOne(photoLink, func(n *html.Node) bool { return isElement(n, "img") }); img != nil {
			avatarURL = attrOr(img, "src")
		}
	}

	return AccountProfile{Username: username, ProfileURL: profileURL, AvatarURL: avatarURL}, true, nil
}
