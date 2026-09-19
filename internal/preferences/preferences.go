// Package preferences persists small, low-stakes app settings (unlike
// internal/playset's irreplaceable user data, a missing or corrupt
// preferences file just means "use defaults") - the first settings file
// this project writes for itself rather than for a game to read, so it
// follows the JSONC house format per CLAUDE.md.
package preferences

import (
	"os"
	"path/filepath"

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
}

// Defaults returns the preferences a fresh install starts with.
func Defaults() Preferences {
	return Preferences{
		ScanForNewMods:       true,
		WarnOnPatchMismatch:  true,
		AutosortDependencies: true,
		AutosortFixesLast:    true,
	}
}

// Load reads path and returns Defaults() on any problem (missing file,
// unreadable, corrupt JSON) - never an error, since falling back to
// defaults is always a safe, sensible response for a settings file.
func Load(path string) Preferences {
	data, err := os.ReadFile(path)
	if err != nil {
		return Defaults()
	}
	var p Preferences
	if err := jsonc.Unmarshal(data, &p); err != nil {
		return Defaults()
	}
	return p
}

// Save writes p to path atomically.
func Save(path string, p Preferences) error {
	dir, filename := filepath.Split(path)
	_, err := atomicfile.WriteJSON(dir, filename, p)
	return err
}
