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
}

// Defaults returns the preferences a fresh install starts with.
func Defaults() Preferences {
	return Preferences{
		ScanForNewMods:      true,
		WarnOnPatchMismatch: true,
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
