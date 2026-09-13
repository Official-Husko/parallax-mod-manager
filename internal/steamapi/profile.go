package steamapi

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
)

// profileURL is a var, not a const, so tests can point it at a local
// httptest.Server. %s is the SteamID64.
var profileURL = "https://steamcommunity.com/profiles/%s/?xml=1"

// Profile is one Steam Community member's real public profile - confirmed
// against a real request (see docs/steam-web-api.md). Only ever fetched
// for a Workshop mod's own creator (PublishedFileDetails.Creator), never
// on its own - this project never looks anyone up who isn't already
// mentioned in real Workshop data it fetched for its own purposes.
type Profile struct {
	Name        string // the "steamID" field - display name, not the numeric id
	AvatarURL   string // avatarMedium - a reasonable size for a small UI badge
	ProfileURL  string // constructed from customURL when set, else /profiles/<id>
	MemberSince string // Steam's own already-formatted string, e.g. "April 30, 2007" - not parsed further
	Location    string
}

// profileEnvelope's XMLName deliberately has no fixed tag - the real
// endpoint's root element differs between a real profile (<profile>) and
// a failure (<response><error>...</error></response>), confirmed
// directly. Leaving XMLName unconstrained lets either decode, and the
// caller tells them apart afterward instead of a decode error.
type profileEnvelope struct {
	XMLName      xml.Name
	Error        string `xml:"error"`
	SteamID64    string `xml:"steamID64"`
	SteamID      string `xml:"steamID"`
	PrivacyState string `xml:"privacyState"`
	AvatarMedium string `xml:"avatarMedium"`
	MemberSince  string `xml:"memberSince"`
	CustomURL    string `xml:"customURL"`
	Location     string `xml:"location"`
}

// GetPlayerProfile fetches one Steam Community member's real public
// profile by their SteamID64. This endpoint has no batching (like
// appdetails, unlike GetPublishedFileDetails) - one request per id;
// callers wanting several should fetch with bounded concurrency, same as
// this project's own dlcstore package does for Store data.
//
// ok is false, with a nil error, for a genuinely failed lookup (a bad id,
// or Steam's own transient "please try again later") - a real, normal
// outcome, not something worth treating as an error. A private profile
// still succeeds here (privacyState is exposed, though a private
// profile's summary/games are simply absent from the fields this project
// reads at all).
func GetPlayerProfile(ctx context.Context, steamID64 string) (profile Profile, ok bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(profileURL, steamID64), nil)
	if err != nil {
		return Profile{}, false, fmt.Errorf("steamapi: building request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return Profile{}, false, fmt.Errorf("steamapi: requesting profile for %s: %w", steamID64, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Profile{}, false, fmt.Errorf("steamapi: profile request for %s failed: HTTP %d", steamID64, resp.StatusCode)
	}

	var envelope profileEnvelope
	if err := xml.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return Profile{}, false, fmt.Errorf("steamapi: decoding profile for %s: %w", steamID64, err)
	}
	if envelope.XMLName.Local != "profile" || envelope.Error != "" || envelope.SteamID == "" {
		return Profile{}, false, nil
	}

	profileURLValue := "https://steamcommunity.com/profiles/" + steamID64
	if envelope.CustomURL != "" {
		profileURLValue = "https://steamcommunity.com/id/" + envelope.CustomURL
	}

	return Profile{
		Name:        envelope.SteamID,
		AvatarURL:   envelope.AvatarMedium,
		ProfileURL:  profileURLValue,
		MemberSince: envelope.MemberSince,
		Location:    envelope.Location,
	}, true, nil
}
