package steamapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// realProfileResponse is (trimmed) a real response for a real Stellaris
// Workshop mod author's public profile, confirmed via a live request
// during development - see docs/steam-web-api.md.
const realProfileResponse = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><profile>
	<steamID64>76561197989629849</steamID64>
	<steamID><![CDATA[Orrie]]></steamID>
	<onlineState>online</onlineState>
	<privacyState>public</privacyState>
	<avatarIcon><![CDATA[https://avatars.akamai.steamstatic.com/x.jpg]]></avatarIcon>
	<avatarMedium><![CDATA[https://avatars.akamai.steamstatic.com/x_medium.jpg]]></avatarMedium>
	<avatarFull><![CDATA[https://avatars.akamai.steamstatic.com/x_full.jpg]]></avatarFull>
	<customURL><![CDATA[orrie]]></customURL>
	<memberSince>April 30, 2007</memberSince>
	<location><![CDATA[Norway]]></location>
</profile>`

// realProfileErrorResponse is a real response for an invalid SteamID64,
// confirmed via a live request - a different root element entirely
// (<response><error>), not a <profile> with an error field.
const realProfileErrorResponse = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><response><error><![CDATA[Failed loading profile data, please try again later.]]></error></response>`

func withProfileServer(t *testing.T, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	restore := profileURL
	profileURL = server.URL + "/?id=%s"
	t.Cleanup(func() { profileURL = restore })
}

func TestGetPlayerProfileParsesRealResponse(t *testing.T) {
	withProfileServer(t, realProfileResponse)

	p, ok, err := GetPlayerProfile(context.Background(), "76561197989629849")
	if err != nil {
		t.Fatalf("GetPlayerProfile: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if p.Name != "Orrie" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.AvatarURL == "" {
		t.Error("AvatarURL is empty, want the real avatarMedium URL")
	}
	if p.ProfileURL != "https://steamcommunity.com/id/orrie" {
		t.Errorf("ProfileURL = %q, want the customURL-based vanity link", p.ProfileURL)
	}
	if p.MemberSince != "April 30, 2007" {
		t.Errorf("MemberSince = %q", p.MemberSince)
	}
	if p.Location != "Norway" {
		t.Errorf("Location = %q", p.Location)
	}
}

func TestGetPlayerProfileFallsBackToProfileURLWithoutCustomURL(t *testing.T) {
	withProfileServer(t, `<profile><steamID64>123</steamID64><steamID><![CDATA[Someone]]></steamID></profile>`)

	p, ok, err := GetPlayerProfile(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetPlayerProfile: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if p.ProfileURL != "https://steamcommunity.com/profiles/123" {
		t.Errorf("ProfileURL = %q, want the numeric-id fallback", p.ProfileURL)
	}
}

func TestGetPlayerProfileInvalidIDReturnsOkFalseNotError(t *testing.T) {
	withProfileServer(t, realProfileErrorResponse)

	_, ok, err := GetPlayerProfile(context.Background(), "00000000000000000")
	if err != nil {
		t.Fatalf("GetPlayerProfile: %v, want no error for a normal failed lookup", err)
	}
	if ok {
		t.Error("ok = true, want false")
	}
}

func TestGetPlayerProfileHTTPErrorSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	restore := profileURL
	profileURL = server.URL + "/?id=%s"
	defer func() { profileURL = restore }()

	if _, _, err := GetPlayerProfile(context.Background(), "1"); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}
