package steamapi

import "testing"

func TestWorkshopPageURL(t *testing.T) {
	got := WorkshopPageURL("123456789")
	want := "https://steamcommunity.com/sharedfiles/filedetails/?id=123456789"
	if got != want {
		t.Errorf("WorkshopPageURL(...) = %q, want %q", got, want)
	}
}

func TestWorkshopClientURL(t *testing.T) {
	got := WorkshopClientURL("123456789")
	want := "steam://url/CommunityFilePage/123456789"
	if got != want {
		t.Errorf("WorkshopClientURL(...) = %q, want %q", got, want)
	}
}
