package steamworks

import "testing"

func TestResolveVersionPicksTheNewestPresentCandidate(t *testing.T) {
	present := map[string]bool{
		"SteamAPI_SteamUGC_v016": true,
		"SteamAPI_SteamUGC_v014": true,
	}
	resolves := func(name string) bool { return present[name] }

	got, ok := resolveVersion(steamUGCVersions, resolves)
	if !ok {
		t.Fatal("resolveVersion reported nothing found, want v016")
	}
	if got != "SteamAPI_SteamUGC_v016" {
		t.Errorf("got %q, want the newest present candidate SteamAPI_SteamUGC_v016 (not the newest candidate overall, and not the oldest present one)", got)
	}
}

func TestResolveVersionFallsBackToAnOlderVersionWhenNewOnesAreMissing(t *testing.T) {
	// Regression case for the real, confirmed mismatch this package exists to
	// handle: a reference header names a newer accessor than a real, older
	// game's library actually exports.
	present := map[string]bool{"SteamAPI_SteamUGC_v016": true}
	resolves := func(name string) bool { return present[name] }

	got, ok := resolveVersion(steamUGCVersions, resolves)
	if !ok || got != "SteamAPI_SteamUGC_v016" {
		t.Errorf("resolveVersion(%v) = %q, %v, want SteamAPI_SteamUGC_v016, true", steamUGCVersions, got, ok)
	}
}

func TestResolveVersionReportsNotFoundWhenNothingResolves(t *testing.T) {
	resolves := func(string) bool { return false }

	_, ok := resolveVersion(steamUtilsVersions, resolves)
	if ok {
		t.Error("resolveVersion reported success against a resolver that never returns true")
	}
}

func TestResolveVersionPicksTheConfirmedFriendsAccessor(t *testing.T) {
	// Regression case for the one real, confirmed data point steamFriendsVersions is built on:
	// nm -D against Stellaris' own bundled libsteam_api.so exports exactly this symbol.
	present := map[string]bool{"SteamAPI_SteamFriends_v017": true}
	resolves := func(name string) bool { return present[name] }

	got, ok := resolveVersion(steamFriendsVersions, resolves)
	if !ok || got != "SteamAPI_SteamFriends_v017" {
		t.Errorf("resolveVersion(%v) = %q, %v, want SteamAPI_SteamFriends_v017, true", steamFriendsVersions, got, ok)
	}
}

func TestResolveVersionNeverProbesPastTheFirstHit(t *testing.T) {
	probed := map[string]int{}
	resolves := func(name string) bool {
		probed[name]++
		return name == steamUtilsVersions[0]
	}

	got, ok := resolveVersion(steamUtilsVersions, resolves)
	if !ok || got != steamUtilsVersions[0] {
		t.Fatalf("resolveVersion = %q, %v, want the first (newest) candidate", got, ok)
	}
	if len(probed) != 1 {
		t.Errorf("probed %d candidates, want exactly 1 (should stop at the first hit): %v", len(probed), probed)
	}
}
