package steamapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// appDetailsURL is a var, not a const, so tests can point it at a local
// httptest.Server. %s is the app id. l=english&cc=us pins a consistent,
// English-formatted release_date and description regardless of wherever
// this app happens to be running from - confirmed directly: the same
// request without these params returned a Turkish-formatted date during
// development, since the endpoint otherwise infers locale from the
// requesting server's own network location.
var appDetailsURL = "https://store.steampowered.com/api/appdetails?appids=%s&l=english&cc=us"

// AppDetails is one Steam Store app's (a game or a DLC) real listing
// details - confirmed against real requests for both a base game and one
// of its DLC, including one genuinely unreleased ("coming soon") DLC (see
// docs/steam-web-api.md). Deliberately doesn't carry price: this project
// only ever fetches Store data for DLC the user has already installed or
// could install, so a price has no use here.
type AppDetails struct {
	Name             string
	ShortDescription string
	HeaderImage      string
	ReleaseDate      string // Steam's own already-localized display string, e.g. "Feb 22, 2018" - not parsed further
	// ComingSoon is Steam's own real, structured flag for "not released
	// yet" - confirmed present (true) on a real unreleased Stellaris DLC,
	// where ReleaseDate is itself just the string "Coming soon" rather
	// than an actual date. Reading this flag directly is more reliable
	// than trying to detect that string, which - unlike release_date's
	// actual dates - isn't guaranteed stable across Steam's own wording
	// changes.
	ComingSoon bool
	// Screenshots lists real preview image URLs (Steam's own
	// "path_thumbnail" size, confirmed present even for a DLC that has no
	// other footprint yet since it isn't released) - the closest thing to
	// "everything Steam has" for content this project can't otherwise
	// show anything about (nothing local to read from at all).
	Screenshots []string
	// DLCAppIDs is a base game's own official DLC catalog (Steam Store
	// field "dlc") - real app ids as strings, confirmed against a real
	// Stellaris request (32 real entries). Empty for a DLC's own
	// AppDetails; only a base game's response carries this field at all.
	DLCAppIDs []string
}

type appDetailsEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Name             string `json:"name"`
		ShortDescription string `json:"short_description"`
		HeaderImage      string `json:"header_image"`
		ReleaseDate      struct {
			ComingSoon bool   `json:"coming_soon"`
			Date       string `json:"date"`
		} `json:"release_date"`
		Screenshots []struct {
			PathThumbnail string `json:"path_thumbnail"`
		} `json:"screenshots"`
		DLC []int `json:"dlc"`
	} `json:"data"`
}

// GetAppDetails fetches one Steam Store app's real details. Confirmed that
// this endpoint does NOT support batching several app ids into one request
// (a comma-separated appids value silently returns a null response,
// verified directly) - callers wanting several need one call per app id,
// same as this project's own dlcstore package does with bounded
// concurrency.
//
// ok is false, with a nil error, when Steam's own "success" field is
// false - a real, normal outcome (a delisted or region-restricted app),
// not a failure worth returning as an error.
func GetAppDetails(ctx context.Context, appID string) (details AppDetails, ok bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(appDetailsURL, appID), nil)
	if err != nil {
		return AppDetails{}, false, fmt.Errorf("steamapi: building request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return AppDetails{}, false, fmt.Errorf("steamapi: requesting app details for %s: %w", appID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AppDetails{}, false, fmt.Errorf("steamapi: app details request for %s failed: HTTP %d", appID, resp.StatusCode)
	}

	// The real shape is {"<appid>": {"success": bool, "data": {...}}} -
	// keyed by the app id we just asked for, which we already know.
	var byAppID map[string]appDetailsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&byAppID); err != nil {
		return AppDetails{}, false, fmt.Errorf("steamapi: decoding app details for %s: %w", appID, err)
	}
	envelope, present := byAppID[appID]
	if !present || !envelope.Success {
		return AppDetails{}, false, nil
	}

	dlcAppIDs := make([]string, 0, len(envelope.Data.DLC))
	for _, id := range envelope.Data.DLC {
		dlcAppIDs = append(dlcAppIDs, strconv.Itoa(id))
	}
	screenshots := make([]string, 0, len(envelope.Data.Screenshots))
	for _, s := range envelope.Data.Screenshots {
		if s.PathThumbnail != "" {
			screenshots = append(screenshots, s.PathThumbnail)
		}
	}

	return AppDetails{
		Name:             envelope.Data.Name,
		ShortDescription: envelope.Data.ShortDescription,
		HeaderImage:      envelope.Data.HeaderImage,
		ReleaseDate:      envelope.Data.ReleaseDate.Date,
		ComingSoon:       envelope.Data.ReleaseDate.ComingSoon,
		Screenshots:      screenshots,
		DLCAppIDs:        dlcAppIDs,
	}, true, nil
}
