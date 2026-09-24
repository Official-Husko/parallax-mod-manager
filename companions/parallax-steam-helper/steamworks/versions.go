package steamworks

// Steamworks bumps each sub-interface's own accessor function name every time
// that interface gains new methods (e.g. SteamAPI_SteamUGC_v016, then _v017,
// and so on) - which exact version a given game ships depends entirely on how
// old its own bundled libsteam_api.so/steam_api64.dll is. Confirmed firsthand:
// Stellaris' own copy exports v016 for ISteamUGC, while a newer reference SDK
// header names v018 for the same interface - a real, live mismatch, not a
// hypothetical one (see the parent project's docs/workshop-upload.md).
//
// Hardcoding one version would silently break against any game whose bundled
// library is older or newer than whatever was tested against - so every
// accessor is resolved by probing known version numbers (newest first)
// against the actual loaded library and using whichever one first resolves,
// never assumed. The individual flat wrapper functions this package calls
// through the resulting interface pointer (CreateItem, SetItemTitle, and so
// on) carry no version suffix of their own and have kept stable parameter
// signatures across every version listed here - only the accessor differs.
//
// These lists only need to reach back far enough to cover Paradox games' own
// oldest still-relevant Steamworks builds, and forward far enough to cover
// what's current - extend them, never shrink them, if a future game's
// library exports something outside this range.
var steamUtilsVersions = []string{
	"SteamAPI_SteamUtils_v012",
	"SteamAPI_SteamUtils_v011",
	"SteamAPI_SteamUtils_v010",
	"SteamAPI_SteamUtils_v009",
}

var steamUGCVersions = []string{
	"SteamAPI_SteamUGC_v020",
	"SteamAPI_SteamUGC_v019",
	"SteamAPI_SteamUGC_v018",
	"SteamAPI_SteamUGC_v017",
	"SteamAPI_SteamUGC_v016",
	"SteamAPI_SteamUGC_v015",
	"SteamAPI_SteamUGC_v014",
}

// steamFriendsVersions: v017 is confirmed firsthand (nm -D against Stellaris'
// own bundled libsteam_api.so on a real Linux install exports exactly
// SteamAPI_SteamFriends_v017); the others are the same defensive
// newest/oldest bracket steamUtilsVersions/steamUGCVersions already keep,
// never confirmed against a real library the way v017 is.
var steamFriendsVersions = []string{
	"SteamAPI_SteamFriends_v018",
	"SteamAPI_SteamFriends_v017",
	"SteamAPI_SteamFriends_v016",
	"SteamAPI_SteamFriends_v015",
}

// resolveVersion returns the first name in candidates (newest first) that
// resolves reports present, given resolves - a thin seam so this probing
// logic is unit-testable without a real library ever being loaded (see
// versions_test.go, which exercises it against a fake symbol table).
func resolveVersion(candidates []string, resolves func(name string) bool) (string, bool) {
	for _, name := range candidates {
		if resolves(name) {
			return name, true
		}
	}
	return "", false
}
