// Package preferences persists small, low-stakes app settings (unlike
// internal/playset's irreplaceable user data, a missing or corrupt
// preferences file just means "use defaults") - the first settings file
// this project writes for itself rather than for a game to read, so it
// follows the JSONC house format per CLAUDE.md.
package preferences

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Preferences holds the app's user-configurable settings.
type Preferences struct {
	// ScanForNewMods gates the live mod-folder watcher (internal/watch):
	// when false, a mod added or removed while the app is open won't show
	// up until the next manual rescan.
	ScanForNewMods bool `json:"scanForNewMods"`
	// CloseAfterLaunch quits the app once LaunchGame succeeds.
	CloseAfterLaunch bool `json:"closeAfterLaunch"`
	// WarnOnPatchMismatch is stored and shown in the UI but not yet acted
	// on - this app has no concept of a mod's compatible game version yet.
	WarnOnPatchMismatch bool `json:"warnOnPatchMismatch"`
	// LastSelectedGame is the game ID to preselect on the next launch.
	LastSelectedGame string `json:"lastSelectedGame"`
	// AutosortDependencies gates Workspace's Autosort rule that moves a mod
	// after every dependency it declares (best-effort name match against
	// currently scanned mods) - see frontend/src/data/autosort.ts.
	AutosortDependencies bool `json:"autosortDependencies"`
	// AutosortFixesLast gates Autosort's rule that moves any mod tagged
	// "Fixes", "Utilities", or "Patch" to the end of the load order -
	// matches the tagging this project's own generated patch mod already
	// uses (see internal/library.GeneratePatch) and real-world modding
	// convention (compatibility patches load last).
	AutosortFixesLast bool `json:"autosortFixesLast"`
	// AutosortPatchLast makes Autosort keep this project's own generated
	// patch mod (see internal/library.GeneratePatch) at the very end of the
	// load order, after everything else, so the resolutions it pins always
	// win. With it off Autosort leaves the patch exactly where the user put
	// it. On by default - see Defaults, and Load for why an older settings
	// file that never had this field still gets that default.
	AutosortPatchLast bool `json:"autosortPatchLast"`
	// ManagedGames is the set of registered game IDs the user has chosen
	// for Parallax Mod Manager to actively manage - only these show up in
	// the game switcher, Library, DLC, and Workspace. nil (or empty) means
	// "no explicit choice made yet" (before the first-run wizard, or a
	// missing/corrupt preferences file), which every consumer of this
	// field treats as "every registered game is visible" rather than
	// "manage nothing".
	ManagedGames []string `json:"managedGames"`
	// GamePaths persists a manually browsed-to install path per game ID
	// (see App.BrowseForGameInstall/BrowseForAnyGameInstall in app.go), so
	// a non-Steam or otherwise auto-detection-missed install is remembered
	// across restarts instead of needing to be re-picked every launch. An
	// entry that no longer verifies (the folder moved or was removed) is
	// treated as unset by its readers rather than trusted blindly.
	GamePaths map[string]string `json:"gamePaths"`
	// ExtraModFolders persists, per game ID, extra folders a user has
	// pointed Parallax Mod Manager at for more mods beyond the game's own
	// managed mod folder (a shared network drive, a manually curated
	// collection, and the like) - see scan.Options.ExtraFolders. Each is
	// searched recursively; only classic-descriptor games are covered.
	ExtraModFolders map[string][]string `json:"extraModFolders"`
	// BackgroundDisabled turns off the rotating per-game background image
	// behind the whole app (see frontend/src/components/AppBackground.tsx)
	// entirely. Named as a negative - "disabled" rather than "enabled" -
	// so an existing preferences file saved before this setting existed
	// unmarshals this field to Go's zero value, false, which then means
	// "not disabled," i.e. still on. An "enabled" field would've silently
	// turned the background off for every existing install the moment
	// this shipped.
	BackgroundDisabled bool `json:"backgroundDisabled"`
	// BackgroundRotationPaused keeps whichever single background image is
	// currently showing instead of picking a new random one every
	// BackgroundIntervalSeconds. Inverted for the same zero-value reason
	// as BackgroundDisabled above.
	BackgroundRotationPaused bool `json:"backgroundRotationPaused"`
	// BackgroundIntervalSeconds is how often a new random background
	// image is picked, in seconds. 0 - including an existing preferences
	// file from before this field existed - means "use the frontend's own
	// default" (5 minutes), handled on read rather than here, since this
	// package's Load only falls back to Defaults() for a missing/corrupt
	// file, not a merely-older one missing just this field.
	BackgroundIntervalSeconds int `json:"backgroundIntervalSeconds"`
	// BackgroundSource is where the background images come from: "online"
	// streams them from the project's GitHub repository as they are needed, and
	// "offline" uses only the copies downloaded into the config folder (see
	// internal/backgrounds). Anything else reads as online.
	BackgroundSource string `json:"backgroundSource"`
	// BackgroundStaticImages remembers, per game ID, the background image (its file name)
	// that Static mode shows for that game, so the same one is loaded every time instead of
	// a random one. It belongs to the backend (App.SetStaticBackground): the frontend holds
	// copies of the preferences and writes them back whole, so App.SetPreferences ignores
	// whatever it sends for this field. A name that is no longer among the game's images
	// (removed, or not downloaded) is replaced by a fresh random pick.
	BackgroundStaticImages map[string]string `json:"backgroundStaticImages"`
	// BackgroundBlur is how strongly the background image is blurred, 0 (sharp, the
	// default) to 100 (the frontend maps that to a blur of up to 24 px). See
	// NormalizedPercent.
	BackgroundBlur int `json:"backgroundBlur"`
	// BackgroundDarken is how strongly the dark layer over the image is drawn, 0 (none)
	// to 100 (almost opaque). DefaultBackgroundDarken is what the app shipped with, so the
	// background looks the same until it is changed - which is why Load decodes onto
	// Defaults(): a file from before this setting keeps 84 instead of reading as 0.
	BackgroundDarken int `json:"backgroundDarken"`
	// LaunchModes persists, per game ID, which of launch.LaunchMode's
	// values LaunchGame should use for that game - kept as a plain string
	// here rather than importing internal/launch's named type, the same
	// way ManagedGames stores raw game IDs rather than a game.GameConfig -
	// this package doesn't need to know what the values mean, only persist
	// them. A missing entry (every existing install, before this setting
	// existed, and any game the user has never configured) reads as
	// launch.LaunchModeSteam - the unchanged, pre-existing behavior.
	LaunchModes map[string]string `json:"launchModes"`
	// LastActivePlaysets persists, per game ID, the playset name most
	// recently saved or explicitly switched to for that game (see
	// Workspace.tsx's rememberActivePlayset) - what PlaysetAutoloadModes'
	// "last" mode (the default) reloads automatically the next time that
	// game is opened, so a returning user doesn't have to reselect it
	// every time. Play itself never depends on a playset being loaded at
	// all (see App.LaunchGame) - launching with nothing selected just
	// launches the game against whatever's already on disk untouched, so
	// this is purely a convenience default, not something correctness
	// depends on. A missing or since-deleted/renamed entry is treated as
	// unset by its one reader (Workspace's own mount effect, which
	// double-checks the name still exists in ListPlaysets before loading
	// it) rather than erroring - Workspace just starts blank.
	LastActivePlaysets map[string]string `json:"lastActivePlaysets"`
	// PlaysetAutoloadModes persists, per game ID, how Workspace should
	// choose which playset (if any) to automatically load when that game
	// is opened: "off" (never autoload - start blank every time, letting
	// Play launch whatever's already on disk untouched until the user
	// explicitly picks or types a name), "last" (LastActivePlaysets for
	// that game), or "custom" (always the specific playset named in
	// PlaysetAutoloadCustom for that game, regardless of what was last
	// active - for a game where you always want the same curated
	// collection loaded even after experimenting with others). An unset
	// entry means "last" - the default, and the behavior this app already
	// had before this setting existed.
	PlaysetAutoloadModes map[string]string `json:"playsetAutoloadModes"`
	// PlaysetAutoloadCustom persists, per game ID, which playset name
	// PlaysetAutoloadModes' "custom" mode should always load for that
	// game - meaningless while that game's mode is "off" or "last". A
	// name that no longer exists (renamed or deleted since) is treated as
	// unset by its one reader (Workspace's own mount effect) rather than
	// erroring, the same as LastActivePlaysets above.
	PlaysetAutoloadCustom map[string]string `json:"playsetAutoloadCustom"`
	// LastSeenGameVersions records, per game ID, the installed version the app
	// last saw - what "the game updated" is judged against (see
	// ObserveGameVersions). It belongs to the backend: the frontend holds
	// copies of the preferences and writes them back whole, so App.SetPreferences
	// ignores whatever it sends for this field.
	LastSeenGameVersions map[string]string `json:"lastSeenGameVersions"`
	// AccentMode says where the interface's main accent colour comes from: "game" (the
	// default) takes each game's own colour from its icon, "custom" uses AccentColor for every
	// game, and "default" keeps the app's own colour. Anything else reads as "game". See
	// NormalizedAccentMode.
	AccentMode string `json:"accentMode"`
	// AccentColor is the custom accent colour as "#rrggbb" (lower case), or "" for none. It
	// only counts while AccentMode is "custom". See NormalizedHexColor.
	AccentColor string `json:"accentColor"`
	// DeveloperTools lets Shift+right-click through to the browser's own menu (with
	// Inspect Element), from Settings > Debug. Off by default, and only development
	// builds (wails dev) have the Debug tab or honour it - a release build ignores
	// it (see devtools.go).
	DeveloperTools bool `json:"developerTools"`
	// LoversLabCheckUpdates gates the periodic check for newer versions of mods
	// installed from LoversLab (see loverslabupdates.go) - on startup and then every
	// LoversLabCheckIntervalHours, same as this app already does once per run for
	// Steam Workshop mods, just repeating. On by default.
	LoversLabCheckUpdates bool `json:"loversLabCheckUpdates"`
	// LoversLabCheckIntervalHours is how often that check repeats while the app is
	// open. DefaultLoversLabCheckIntervalHours is what the app shipped with.
	LoversLabCheckIntervalHours int `json:"loversLabCheckIntervalHours"`
	// LoversLabNotifications gates the periodic check for the signed-in LoversLab
	// account's own real site notifications (see loverslab.go's
	// LoversLabUnreadNotifications) - a separate, much shorter interval from the mod
	// update check above, since a notification (a reply, a reaction) is worth
	// noticing sooner than a mod update is. On by default.
	LoversLabNotifications bool `json:"loversLabNotifications"`
	// LoversLabNotificationIntervalMinutes is how often that check repeats while the
	// app is open. DefaultLoversLabNotificationIntervalMinutes is what the app
	// shipped with.
	LoversLabNotificationIntervalMinutes int `json:"loversLabNotificationIntervalMinutes"`
	// ShareToolMark gates internal/toolmark: when true, publishing a mod to Steam
	// Workshop writes a small PARALLAX_TOOLS.md into it first, noting it was made
	// with this app and linking back to the project - purely to help the project
	// itself be found by anyone who downloads the mod. Off by default (Go's own
	// zero value for a bool already means "off", so an existing settings file
	// saved before this setting existed reads as off, never silently on) - see
	// ToolMarkPromptShown for the one-time "Allow / No thanks" prompt that offers
	// to turn it on.
	ShareToolMark bool `json:"shareToolMark"`
	// ToolMarkPromptShown is true once the user has answered (or dismissed) the
	// one-time prompt offering ShareToolMark, on their first-ever Workshop
	// publish - it is never shown again after that, whichever way they answered.
	ToolMarkPromptShown bool `json:"toolMarkPromptShown"`
	// FeatureBrowseEnabled/FeatureEditorEnabled/FeatureLibraryEnabled gate a whole
	// area of the app off entirely - Settings > Features (and the first-run
	// wizard's own Preferences step) let a person turn off ones they don't use,
	// hiding that area's own top-nav tab. Browse is the only one of these three
	// with a real background task of its own (its periodic LoversLab update and
	// notification checks - see app.tsx), stopped the same way turning either of
	// those off in Settings > Browse already stops it, just gated on this too. On
	// by default - see Defaults, and Load's own doc comment for why an existing
	// settings file saved before these existed still reads them as on rather
	// than silently defaulting to off the moment this shipped.
	FeatureBrowseEnabled  bool `json:"featureBrowseEnabled"`
	FeatureEditorEnabled  bool `json:"featureEditorEnabled"`
	FeatureLibraryEnabled bool `json:"featureLibraryEnabled"`
	// FeatureConflictsEnabled gates conflict detection itself, not a nav
	// tab (there isn't one) - turning it off skips conflict.Resolve
	// entirely during a scan, so Summary.Conflicts/Summary.Patch stay
	// empty and Generate Patch has nothing to write. Marked EXPERIMENTAL
	// in Settings > Features: conflict detection went through a large,
	// same-day round of real bug fixes and new behavior (see
	// docs/conflict-resolution.md), and this is the escape hatch if any of
	// it misbehaves for someone before it's had more real-world mileage.
	// On by default, same reasoning as the three above.
	FeatureConflictsEnabled bool `json:"featureConflictsEnabled"`
	// WorkshopOpenMode is a person's chosen way of opening a Workshop
	// item's page from this app - steamapi.WorkshopOpenMode's own two
	// values ("app", the default, which opens the local Steam client
	// directly via its own steam:// protocol handler, or "browser", used
	// either by explicit choice or automatically as "app"'s own fallback
	// if opening Steam fails), stored as a plain string here the same way
	// LaunchModes already is, so this package doesn't need to import
	// steamapi just to hold a settings value. An empty string (an
	// existing settings file saved before this setting existed, or saved
	// while "app" was still the implicit zero value rather than a real
	// default) is treated as "app" wherever this is read, not as an
	// invalid value.
	WorkshopOpenMode string `json:"workshopOpenMode"`
}

// DefaultLoversLabCheckIntervalHours is what the app shipped with - see
// Preferences.LoversLabCheckIntervalHours.
const DefaultLoversLabCheckIntervalHours = 4

// DefaultLoversLabNotificationIntervalMinutes is what the app shipped with - see
// Preferences.LoversLabNotificationIntervalMinutes.
const DefaultLoversLabNotificationIntervalMinutes = 10

// VersionChange is one game whose installed version differs from the last one
// seen.
type VersionChange struct {
	GameID string
	From   string
	To     string
}

// ObserveGameVersions records current (game ID -> installed version) as the
// versions last seen, and reports which games changed from a version seen
// before. A game seen for the first time is only recorded - there was nothing
// to update from - and an empty version (the game isn't installed, or reports
// none) is ignored entirely: never a change, never recorded, so a game that
// briefly can't be found doesn't turn into a false "updated" when it returns.
// Changes are in game ID order. p itself isn't modified.
func (p Preferences) ObserveGameVersions(current map[string]string) (Preferences, []VersionChange) {
	seen := make(map[string]string, len(p.LastSeenGameVersions)+len(current))
	for k, v := range p.LastSeenGameVersions {
		seen[k] = v
	}
	ids := make([]string, 0, len(current))
	for id := range current {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var changes []VersionChange
	for _, id := range ids {
		version := current[id]
		if version == "" {
			continue
		}
		if was := seen[id]; was != "" && was != version {
			changes = append(changes, VersionChange{GameID: id, From: was, To: version})
		}
		seen[id] = version
	}
	p.LastSeenGameVersions = seen
	return p, changes
}

// The two values of Preferences.BackgroundSource.
const (
	BackgroundSourceOnline  = "online"
	BackgroundSourceOffline = "offline"
)

// NormalizedBackgroundSource returns "offline" for exactly that and "online" for
// anything else, so a hand-edited or missing value can never mean neither.
func NormalizedBackgroundSource(v string) string {
	if v == BackgroundSourceOffline {
		return BackgroundSourceOffline
	}
	return BackgroundSourceOnline
}

// The values of Preferences.AccentMode.
const (
	AccentModeGame    = "game"
	AccentModeCustom  = "custom"
	AccentModeDefault = "default"
)

// NormalizedAccentMode returns v when it is one of the three modes and "game" for anything else
// (a missing or hand-edited value can never mean none of them).
func NormalizedAccentMode(v string) string {
	switch v {
	case AccentModeCustom, AccentModeDefault:
		return v
	}
	return AccentModeGame
}

// NormalizedHexColor returns v as "#rrggbb" in lower case when it is a hex colour (#rgb or
// #rrggbb, the # optional), and "" when it is not.
func NormalizedHexColor(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "#")
	if len(v) == 3 {
		v = string([]byte{v[0], v[0], v[1], v[1], v[2], v[2]})
	}
	if len(v) != 6 {
		return ""
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return ""
		}
	}
	return "#" + strings.ToLower(v)
}

// DefaultBackgroundDarken is the darkening of the background image the app has always
// had (a scrim fading from 80% to 88% black), as a strength out of 100.
const DefaultBackgroundDarken = 84

// NormalizedPercent clamps a 0-100 setting: a hand-edited value can never mean
// something the sliders cannot show.
func NormalizedPercent(v int) int {
	return min(100, max(0, v))
}

// normalized returns p with its numeric settings brought into range.
func (p Preferences) normalized() Preferences {
	p.BackgroundBlur = NormalizedPercent(p.BackgroundBlur)
	p.BackgroundDarken = NormalizedPercent(p.BackgroundDarken)
	p.AccentMode = NormalizedAccentMode(p.AccentMode)
	p.AccentColor = NormalizedHexColor(p.AccentColor)
	return p
}

// Defaults returns the preferences a fresh install starts with.
func Defaults() Preferences {
	return Preferences{
		ScanForNewMods:                       true,
		WarnOnPatchMismatch:                  true,
		AutosortDependencies:                 true,
		AutosortFixesLast:                    true,
		AutosortPatchLast:                    true,
		BackgroundSource:                     BackgroundSourceOnline,
		BackgroundDarken:                     DefaultBackgroundDarken,
		AccentMode:                           AccentModeGame,
		LoversLabCheckUpdates:                true,
		LoversLabCheckIntervalHours:          DefaultLoversLabCheckIntervalHours,
		LoversLabNotifications:               true,
		LoversLabNotificationIntervalMinutes: DefaultLoversLabNotificationIntervalMinutes,
		FeatureBrowseEnabled:                 true,
		FeatureEditorEnabled:                 true,
		FeatureLibraryEnabled:                true,
		FeatureConflictsEnabled:              true,
		WorkshopOpenMode:                     "app",
	}
}

// Load reads path and returns Defaults() on any problem (missing file,
// unreadable, corrupt JSON) - never an error, since falling back to
// defaults is always a safe, sensible response for a settings file.
//
// The file is decoded *onto* Defaults(), not onto a zero value: a setting
// the file doesn't mention (one added after it was saved) keeps its default
// instead of silently reading as false. That matters for every setting that
// is on by default - an older file would otherwise have quietly switched
// them all off the day a new version shipped. Every field the file does
// contain still wins, so an explicit false stays false.
func Load(path string) Preferences {
	data, err := os.ReadFile(path)
	if err != nil {
		return Defaults()
	}
	p := Defaults()
	if err := jsonc.Unmarshal(data, &p); err != nil {
		return Defaults()
	}
	return p.normalized()
}

// Save writes p to path atomically.
func Save(path string, p Preferences) error {
	dir, filename := filepath.Split(path)
	_, err := atomicfile.WriteJSON(dir, filename, p)
	return err
}

// WithPlaysetRenamed returns p with every reference to gameID's playset oldName
// pointing at newName instead: the playset remembered as last active, and the
// one pinned for auto-loading. Playsets are referred to by name in settings, so
// a rename that didn't do this would leave them pointing at nothing. p itself
// isn't modified - its maps may be shared with other copies.
func (p Preferences) WithPlaysetRenamed(gameID, oldName, newName string) Preferences {
	p.LastActivePlaysets = replacedEntry(p.LastActivePlaysets, gameID, oldName, newName, false)
	p.PlaysetAutoloadCustom = replacedEntry(p.PlaysetAutoloadCustom, gameID, oldName, newName, false)
	return p
}

// WithoutPlayset returns p with every reference to gameID's playset name
// removed - what a deleted playset should leave behind. (A game with no last
// active or pinned playset just falls back to its default, "start blank".)
func (p Preferences) WithoutPlayset(gameID, name string) Preferences {
	p.LastActivePlaysets = replacedEntry(p.LastActivePlaysets, gameID, name, "", true)
	p.PlaysetAutoloadCustom = replacedEntry(p.PlaysetAutoloadCustom, gameID, name, "", true)
	return p
}

// replacedEntry returns m with m[key] changed from old to new (or deleted, if
// remove) when it currently equals old, as a copy - m is returned as-is when
// there's nothing to change.
func replacedEntry(m map[string]string, key, old, new string, remove bool) map[string]string {
	if m[key] != old || old == "" {
		return m
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	if remove {
		delete(out, key)
	} else {
		out[key] = new
	}
	return out
}

// ChangedKeys names the settings that differ between two Preferences, using
// their JSON names (the ones a person sees in preferences.jsonc), in field
// order - what the activity log records when settings are saved, so "preferences
// saved" says which ones. Settings that are maps or lists count as one setting
// each, changed if any entry is.
func ChangedKeys(before, after Preferences) []string {
	var keys []string
	bv, av := reflect.ValueOf(before), reflect.ValueOf(after)
	t := bv.Type()
	for i := 0; i < t.NumField(); i++ {
		if sameSetting(bv.Field(i), av.Field(i)) {
			continue
		}
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" {
			name = t.Field(i).Name
		}
		keys = append(keys, name)
	}
	return keys
}

// sameSetting compares one setting's old and new value. An empty map or list
// equals a nil one: preferences travel through JSON, where "nothing set" turns
// up as either depending on which side last touched it, and that isn't a change
// anyone made.
func sameSetting(a, b reflect.Value) bool {
	switch a.Kind() {
	case reflect.Map, reflect.Slice:
		if a.Len() == 0 && b.Len() == 0 {
			return true
		}
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}
