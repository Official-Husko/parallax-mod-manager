package steamapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// itemPageURL is a var, not a const, so tests can point it at a local
// httptest.Server. %s is the published file id.
var itemPageURL = "https://steamcommunity.com/sharedfiles/filedetails/?id=%s"

// maxItemPageBytes bounds how much of an item page is read - the markers looked
// for sit near the top, and a Workshop page with a long description is far
// larger than it needs to be to find them.
const maxItemPageBytes = 1 << 20

// ItemPageExists says whether a Workshop item's own public page is there.
//
// GetPublishedFileDetails is not enough to tell that an item was deleted: it
// answers "not found" (result 9) for some items whose page is up and readable -
// confirmed against a real one an author had retitled "OUTDATED ..." - so
// "not found" there only means "look at the page". A live page carries the
// item's title; a missing one is Steam's generic "There was a problem accessing
// the item" error page.
//
// Anything else - a request that failed, an HTTP status other than 200, a page
// that is neither (a login or age-check wall) - is an error, not an answer:
// callers must treat it as "unknown" and never as "deleted".
func ItemPageExists(ctx context.Context, publishedFileID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(itemPageURL, publishedFileID), nil)
	if err != nil {
		return false, fmt.Errorf("steamapi: building request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("steamapi: requesting item page for %s: %w", publishedFileID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("steamapi: item page request for %s failed: HTTP %d", publishedFileID, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxItemPageBytes))
	if err != nil {
		return false, fmt.Errorf("steamapi: reading item page for %s: %w", publishedFileID, err)
	}
	page := string(body)
	switch {
	case strings.Contains(page, `class="workshopItemTitle"`):
		return true, nil
	case strings.Contains(page, "There was a problem accessing the item"):
		return false, nil
	default:
		return false, fmt.Errorf("steamapi: item page for %s is neither an item nor Steam's not-found page", publishedFileID)
	}
}
