// Package priorityrules persists a user's own per-Type conflict priority
// overrides - "for this exact Type, use FIOS (or LIOS) instead of the
// built-in default" - see conflict.DefaultPriorityRules and
// docs/conflict-resolution.md.
//
// The built-in default list is deliberately small: it only ever grows from
// something this project has itself confirmed against a real source (see
// that var's own doc comment), never a guess. This store is the other side
// of that same caution - rather than the app centrally guessing at which
// object Types need FIOS for a given game, a person who already knows from
// their own modding experience that a specific Type needs it can say so
// themselves, scoped to their own install.
//
// Low-stakes, re-derivable data (a missing or corrupt file just means every
// Type falls back to the built-in default, exactly as if no override were
// set) - so this follows internal/patchoverride's own "never a hard error,
// degrade to empty" philosophy, one file per game for the same reason
// patchoverride is: conflict resolution itself is per-game, not per
// playset, so an override tied to one playset would be meaningless once a
// different one is active.
package priorityrules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// FIOS and LIOS are this store's own on-disk spelling of
// conflict.FIOS/conflict.LIOS - kept as plain strings here (rather than
// importing internal/conflict's own enum) so this low-level package stays
// as decoupled from conflict as internal/patchoverride already is; the
// caller (internal/app) is what actually converts these into
// conflict.PriorityRule values.
const (
	FIOS = "FIOS"
	LIOS = "LIOS"
)

type fileFormat struct {
	// Rules maps a definition.Type (a literal mod folder path, e.g.
	// "common/static_modifiers") to FIOS or LIOS.
	Rules map[string]string `json:"rules"`
}

// Store persists overrides on disk, one file per game.
type Store struct {
	Dir string
}

func (s Store) path(gameID string) string {
	return filepath.Join(s.Dir, gameID+".jsonc")
}

// Load returns gameID's saved per-Type overrides, or an empty (non-nil)
// map for a missing, unreadable, or corrupt file - never an error, the
// same reasoning patchoverride.Store.Load already documents.
func (s Store) Load(gameID string) map[string]string {
	if s.Dir == "" {
		return map[string]string{}
	}
	data, err := os.ReadFile(s.path(gameID))
	if err != nil {
		return map[string]string{}
	}
	var f fileFormat
	if err := jsonc.Unmarshal(data, &f); err != nil || f.Rules == nil {
		return map[string]string{}
	}
	return f.Rules
}

// Save atomically writes gameID's full override set, replacing whatever
// was there before.
func (s Store) Save(gameID string, rules map[string]string) error {
	if s.Dir == "" {
		return errors.New("priorityrules: Store dir must be set explicitly")
	}
	if _, err := atomicfile.WriteJSON(s.Dir, filepath.Base(s.path(gameID)), fileFormat{Rules: rules}); err != nil {
		return fmt.Errorf("priorityrules: saving rules for %q: %w", gameID, err)
	}
	return nil
}
