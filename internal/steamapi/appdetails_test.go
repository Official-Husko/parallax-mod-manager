package steamapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// realDLCAppDetailsResponse is (trimmed) a real response for a Stellaris
// DLC app id with l=english&cc=us, confirmed via a live request during
// development - see docs/steam-web-api.md.
const realDLCAppDetailsResponse = `{"716670":{"success":true,"data":{"type":"dlc","name":"Stellaris: Apocalypse","short_description":"A full expansion which redefines stellar warfare.","header_image":"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/716670/header.jpg?t=1732009886","is_free":false,"release_date":{"coming_soon":false,"date":"Feb 22, 2018"},"price_overview":{"currency":"USD","initial":1399,"final":1399,"discount_percent":0,"initial_formatted":"","final_formatted":"$13.99 USD"},"fullgame":{"appid":"281990","name":"Stellaris"}}}}`

func withAppDetailsServer(t *testing.T, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	restore := appDetailsURL
	appDetailsURL = server.URL + "/?appids=%s"
	t.Cleanup(func() { appDetailsURL = restore })
}

func TestGetAppDetailsParsesRealDLCResponse(t *testing.T) {
	withAppDetailsServer(t, realDLCAppDetailsResponse)

	d, ok, err := GetAppDetails(context.Background(), "716670")
	if err != nil {
		t.Fatalf("GetAppDetails: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true for a successful response")
	}
	if d.Name != "Stellaris: Apocalypse" {
		t.Errorf("Name = %q", d.Name)
	}
	if d.HeaderImage == "" {
		t.Error("HeaderImage is empty, want the real image URL")
	}
	if d.ReleaseDate != "Feb 22, 2018" {
		t.Errorf("ReleaseDate = %q", d.ReleaseDate)
	}
}

// realComingSoonDLCResponse is (trimmed) a real response for a genuinely
// unreleased Stellaris DLC, confirmed via a live request during
// development - see docs/steam-web-api.md.
const realComingSoonDLCResponse = `{"4241450":{"success":true,"data":{"type":"dlc","name":"Stellaris: Scenario Pack 1","short_description":"Embark on a new set of galactic challenges!","header_image":"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/4241450/header.jpg","screenshots":[{"id":0,"path_thumbnail":"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/4241450/ss1.600x338.jpg","path_full":"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/4241450/ss1.1920x1080.jpg"},{"id":1,"path_thumbnail":"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/4241450/ss2.600x338.jpg","path_full":"https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/4241450/ss2.1920x1080.jpg"}],"release_date":{"coming_soon":true,"date":"Coming soon"}}}}`

func TestGetAppDetailsParsesComingSoonDLC(t *testing.T) {
	withAppDetailsServer(t, realComingSoonDLCResponse)

	d, ok, err := GetAppDetails(context.Background(), "4241450")
	if err != nil {
		t.Fatalf("GetAppDetails: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !d.ComingSoon {
		t.Error("ComingSoon = false, want true")
	}
	if d.ReleaseDate != "Coming soon" {
		t.Errorf("ReleaseDate = %q", d.ReleaseDate)
	}
	if len(d.Screenshots) != 2 {
		t.Fatalf("Screenshots = %v, want 2 real thumbnail URLs", d.Screenshots)
	}
	for _, s := range d.Screenshots {
		if s == "" {
			t.Error("Screenshots contains an empty URL")
		}
	}
}

func TestGetAppDetailsReleasedAppHasComingSoonFalse(t *testing.T) {
	withAppDetailsServer(t, realDLCAppDetailsResponse)

	d, _, err := GetAppDetails(context.Background(), "716670")
	if err != nil {
		t.Fatalf("GetAppDetails: %v", err)
	}
	if d.ComingSoon {
		t.Error("ComingSoon = true, want false for an already-released DLC")
	}
}

func TestGetAppDetailsParsesBaseGameDLCCatalog(t *testing.T) {
	withAppDetailsServer(t, `{"281990":{"success":true,"data":{"name":"Stellaris","dlc":[4241400,4241420,716670]}}}`)

	d, ok, err := GetAppDetails(context.Background(), "281990")
	if err != nil {
		t.Fatalf("GetAppDetails: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	want := []string{"4241400", "4241420", "716670"}
	if len(d.DLCAppIDs) != len(want) {
		t.Fatalf("DLCAppIDs = %v, want %v", d.DLCAppIDs, want)
	}
	for i, id := range want {
		if d.DLCAppIDs[i] != id {
			t.Errorf("DLCAppIDs[%d] = %q, want %q", i, d.DLCAppIDs[i], id)
		}
	}
}

func TestGetAppDetailsDLCResponseHasNoDLCList(t *testing.T) {
	// A DLC's own AppDetails response never carries a "dlc" field at all -
	// confirmed against the real DLC response fixture above.
	withAppDetailsServer(t, realDLCAppDetailsResponse)

	d, _, err := GetAppDetails(context.Background(), "716670")
	if err != nil {
		t.Fatalf("GetAppDetails: %v", err)
	}
	if len(d.DLCAppIDs) != 0 {
		t.Errorf("DLCAppIDs = %v, want empty for a DLC's own response", d.DLCAppIDs)
	}
}

func TestGetAppDetailsUnsuccessfulReturnsOkFalseNotError(t *testing.T) {
	withAppDetailsServer(t, `{"12345":{"success":false}}`)

	_, ok, err := GetAppDetails(context.Background(), "12345")
	if err != nil {
		t.Fatalf("GetAppDetails: %v, want no error for a normal unsuccessful lookup", err)
	}
	if ok {
		t.Error("ok = true, want false")
	}
}

func TestGetAppDetailsHTTPErrorSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	restore := appDetailsURL
	appDetailsURL = server.URL + "/?appids=%s"
	defer func() { appDetailsURL = restore }()

	if _, _, err := GetAppDetails(context.Background(), "1"); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}
