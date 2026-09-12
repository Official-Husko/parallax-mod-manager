package launch

import "testing"

func TestSteamAppURLNoArgs(t *testing.T) {
	got := SteamAppURL(testClassicGame(), nil)
	want := "steam://run/281990"
	if got != want {
		t.Errorf("SteamAppURL = %q, want %q", got, want)
	}
}

func TestSteamAppURLOneArg(t *testing.T) {
	got := SteamAppURL(testClassicGame(), []string{"-skiplauncher"})
	want := "steam://run/281990/-skiplauncher"
	if got != want {
		t.Errorf("SteamAppURL = %q, want %q", got, want)
	}
}

func TestSteamAppURLMultipleArgsSpaceJoined(t *testing.T) {
	got := SteamAppURL(testClassicGame(), []string{"-skiplauncher", "-debug"})
	want := "steam://run/281990/-skiplauncher -debug"
	if got != want {
		t.Errorf("SteamAppURL = %q, want %q", got, want)
	}
}

func TestSteamAppURLEmptyAppID(t *testing.T) {
	cfg := testClassicGame()
	cfg.SteamAppID = ""
	got := SteamAppURL(cfg, nil)
	want := "steam://run/"
	if got != want {
		t.Errorf("SteamAppURL = %q, want %q (Launch is what guards against calling this with no app id)", got, want)
	}
}
