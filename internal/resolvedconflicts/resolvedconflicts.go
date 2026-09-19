// Package resolvedconflicts persists which contested conflict keys a user
// has manually marked as reviewed - a personal "I've looked at this, it's
// fine" bookkeeping flag only, distinct from an actual patch override
// (internal/patchoverride): marking a key resolved doesn't change which
// mod wins it, it just tells the Conflict Resolver's own contested-keys
// list to stop flagging that one in red, the same way an acknowledged
// warning elsewhere in this app quiets the flag without actually fixing
// anything underneath it. One file per game, keyed the same "Type:ID" way
// patchoverride.Key already does, so the two agree on identity without
// either needing to know about the other's implementation.
//
// Low-stakes, re-derivable data (worst case on a missing or corrupt file:
// every conflict just shows as unresolved again, exactly like before this
// feature existed) - follows internal/preferences' own "never a hard
// error, degrade to empty" philosophy rather than internal/playset's "a
// corrupt file is a real error" one. Written as JSONC per CLAUDE.md, so a
// user can add their own comment next to a key they marked and why.
package resolvedconflicts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Key returns the identifier one conflict is tracked under - the same
// "Type:ID" convention patchoverride.Key and the frontend's own
// conflictKey helper already use.
func Key(conflictType, conflictID string) string {
	return conflictType + ":" + conflictID
}

type fileFormat struct {
	// Keys lists every conflict (see Key) currently marked resolved for
	// this game.
	Keys []string `json:"keys"`
}

// Store persists a resolved-conflicts set on disk, one file per game.
type Store struct {
	Dir string
}

func (s Store) path(gameID string) string {
	return filepath.Join(s.Dir, gameID+".jsonc")
}

// Load returns gameID's saved resolved-key set, or an empty (non-nil)
// slice for a missing, unreadable, or corrupt file - never an error,
// matching versionignore.Load's own reasoning: falling back to "nothing
// marked yet" is always a safe, sensible response for this kind of
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
	if err := jsonc.Unmarshal(data, &f); err != nil || f.Keys == nil {
		return []string{}
	}
	return f.Keys
}

// Save atomically writes gameID's full resolved-key set, replacing
// whatever was there before. keys is sorted before writing so the file
// stays diff-friendly across saves.
func (s Store) Save(gameID string, keys []string) error {
	if s.Dir == "" {
		return fmt.Errorf("resolvedconflicts: Store dir must be set explicitly")
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	if _, err := atomicfile.WriteJSON(s.Dir, filepath.Base(s.path(gameID)), fileFormat{Keys: sorted}); err != nil {
		return fmt.Errorf("resolvedconflicts: saving resolved set for %q: %w", gameID, err)
	}
	return nil
}
