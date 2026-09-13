// Package patchoverride persists manual per-conflict winner overrides -
// a user's explicit choice to make a specific mod win a specific
// contested key, instead of whichever mod library.GeneratePatch's own
// automatic load-order rule (LIOS/FIOS) would otherwise pick. See
// docs/patch-mods.md.
//
// Low-stakes, re-derivable data (worst case on a missing or corrupt file:
// every conflict just falls back to its automatic winner, exactly like
// before this feature existed) - so this follows internal/preferences'
// own "never a hard error, degrade to empty" philosophy rather than
// internal/playset's "a corrupt file is a real error" one. One file per
// game, since patch generation itself is one patch per game, not per
// playset (see docs/patch-mods.md) - a manual override that named a
// specific playset would be meaningless once a different one is active.
// Written as JSONC per CLAUDE.md, so a user can add their own comments
// next to a choice they want to remember why they made.
package patchoverride

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Key returns the map key one conflict's manual override is stored and
// looked up under - the same "Type:ID" convention the frontend's own
// conflictKey helper already uses for the identical purpose, so both
// sides agree on it without either needing to know about the other's
// implementation.
func Key(conflictType, conflictID string) string {
	return conflictType + ":" + conflictID
}

type fileFormat struct {
	// Overrides maps a conflict Key to the mod ID manually chosen to win
	// it. A conflict absent from this map uses the automatic load-order
	// winner - the default for every conflict until a user overrides it.
	Overrides map[string]string `json:"overrides"`
}

// Store persists overrides on disk, one file per game.
type Store struct {
	Dir string
}

func (s Store) path(gameID string) string {
	return filepath.Join(s.Dir, gameID+".jsonc")
}

// Load returns gameID's saved overrides, or an empty (non-nil) map for a
// missing, unreadable, or corrupt file - never an error, matching
// preferences.Load's own reasoning: falling back to "no overrides yet" is
// always a safe, sensible response for this kind of settings file.
func (s Store) Load(gameID string) map[string]string {
	if s.Dir == "" {
		return map[string]string{}
	}
	data, err := os.ReadFile(s.path(gameID))
	if err != nil {
		return map[string]string{}
	}
	var f fileFormat
	if err := jsonc.Unmarshal(data, &f); err != nil || f.Overrides == nil {
		return map[string]string{}
	}
	return f.Overrides
}

// Save atomically writes gameID's full override set, replacing whatever
// was there before.
func (s Store) Save(gameID string, overrides map[string]string) error {
	if s.Dir == "" {
		return fmt.Errorf("patchoverride: Store dir must be set explicitly")
	}
	if _, err := atomicfile.WriteJSON(s.Dir, filepath.Base(s.path(gameID)), fileFormat{Overrides: overrides}); err != nil {
		return fmt.Errorf("patchoverride: saving overrides for %q: %w", gameID, err)
	}
	return nil
}
