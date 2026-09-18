// Package versionignore persists which mods a user has chosen to ignore
// the "supported_version doesn't match the installed game version"
// warning for - a deliberate, informed "yes I know, stop flagging this"
// choice per mod, kept until the user reverses it (see the Workspace
// mod context menu's own "Ignore incompatibility"/"Stop ignoring
// incompatibility" pair in app.go). One file per game, matching
// internal/patchoverride's own reasoning: this is mod-identity-scoped,
// not tied to any one playset.
//
// Low-stakes, re-derivable data (worst case on a missing or corrupt file:
// every ignored mod's warning just shows again) - follows
// internal/preferences' own "never a hard error, degrade to empty"
// philosophy rather than internal/playset's "a corrupt file is a real
// error" one. Written as JSONC per CLAUDE.md, so a user can add their own
// comment next to a mod they chose to ignore and why.
package versionignore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

type fileFormat struct {
	// ModIDs lists every mod, by ID, whose version-incompatibility warning
	// is currently suppressed for this game.
	ModIDs []string `json:"modIds"`
}

// Store persists an ignored-incompatibility set on disk, one file per
// game.
type Store struct {
	Dir string
}

func (s Store) path(gameID string) string {
	return filepath.Join(s.Dir, gameID+".jsonc")
}

// Load returns gameID's saved ignored-mod-ID set, or an empty (non-nil)
// slice for a missing, unreadable, or corrupt file - never an error,
// matching preferences.Load's own reasoning: falling back to "nothing
// ignored yet" is always a safe, sensible response for this kind of
// settings file.
func (s Store) Load(gameID string) []string {
	if s.Dir == "" {
		return []string{}
	}
	data, err := os.ReadFile(s.path(gameID))
	if err != nil {
		return []string{}
	}
	var f fileFormat
	if err := jsonc.Unmarshal(data, &f); err != nil || f.ModIDs == nil {
		return []string{}
	}
	return f.ModIDs
}

// Save atomically writes gameID's full ignored set, replacing whatever
// was there before. ids is sorted before writing so the file stays
// diff-friendly across saves.
func (s Store) Save(gameID string, ids []string) error {
	if s.Dir == "" {
		return fmt.Errorf("versionignore: Store dir must be set explicitly")
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	if _, err := atomicfile.WriteJSON(s.Dir, filepath.Base(s.path(gameID)), fileFormat{ModIDs: sorted}); err != nil {
		return fmt.Errorf("versionignore: saving ignored set for %q: %w", gameID, err)
	}
	return nil
}
