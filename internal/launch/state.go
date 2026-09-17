// Package launch writes a game's own on-disk load-order state (which mods
// are enabled, in what order) and launches the game the way Steam/the
// Paradox Launcher would - a steam://run/<appid> URL, not a -mod= flag. See
// docs/game-launching.md.
//
// Safety: WriteState never resolves game.GameConfig.UserDataDir() itself -
// Options.StateDir is a required, explicit parameter with no fallback.
// Resolving the real on-disk path is the composition layer's job (a future
// app.go), so this package (and its own tests) can never accidentally
// write into a real installed game.
package launch

import (
	"errors"
	"fmt"
	"path"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// Options configures a WriteState call.
type Options struct {
	// StateDir is the directory dlc_load.json is written into. Required -
	// WriteState returns ErrStateDirRequired if empty, before touching the
	// filesystem at all.
	StateDir string

	// DisabledDLC is written to dlc_load.json's disabled_dlcs field.
	// DLC enable/disable decisions themselves are out of scope here; this
	// only serializes whatever the caller already decided. A nil slice is
	// written as an empty list, not "leave unchanged."
	DisabledDLC []string
}

// Result reports what WriteState actually wrote, for logging/UI feedback.
type Result struct {
	Written []string // absolute paths, in the order they were written
}

var (
	// ErrStateDirRequired means Options.StateDir was empty.
	ErrStateDirRequired = errors.New("launch: Options.StateDir must be set explicitly")

	// ErrUnsupportedDescriptorType means cfg.DescriptorType isn't
	// mod.DescriptorClassic yet - JSON-launcher-format games
	// (content_load.json/playsets.json) are a documented, not-yet-built
	// extension point (docs/game-launching.md doesn't confirm their
	// field-level shape).
	ErrUnsupportedDescriptorType = errors.New("launch: descriptor type not yet supported")
)

// UnknownModsError means order referenced one or more mod IDs not present
// in mods - WriteState refuses to write a state file that references a mod
// that was never actually scanned, rather than writing something the game
// can't resolve.
type UnknownModsError struct {
	IDs []string
}

func (e *UnknownModsError) Error() string {
	return fmt.Sprintf("launch: load order references mod(s) not present in mods: %v", e.IDs)
}

// WriteState validates order against mods, builds this game's on-disk
// enabled-mods representation per cfg.DescriptorType, and writes it
// atomically under opts.StateDir.
func WriteState(order conflict.LoadOrder, mods []mod.Mod, cfg game.GameConfig, opts Options) (Result, error) {
	if opts.StateDir == "" {
		return Result{}, ErrStateDirRequired
	}
	if cfg.DescriptorType != mod.DescriptorClassic {
		return Result{}, ErrUnsupportedDescriptorType
	}

	enabledMods, err := resolveEnabledModList(order, mods)
	if err != nil {
		return Result{}, err
	}

	return writeClassicState(enabledMods, opts)
}

// classicDescriptorPath is the descriptor path a classic-format game
// expects for mod id - "mod/<id>.mod", always forward-slashed (this is
// data inside a JSON file the Clausewitz engine parses, not a native OS
// path: using filepath.Join here would silently emit backslashes on a
// Windows build and produce a file the game can't resolve). Both
// dlc_load.json's enabled_mods and mods_registry.json's gameRegistryId use
// exactly this same string - it's the bridge between the two identifier
// spaces (see docs/game-launching.md).
func classicDescriptorPath(id string) string {
	return path.Join("mod", id+".mod")
}

// resolveEnabledModList deduplicates order (first occurrence wins,
// mirroring conflict.LoadOrder.Priority's own tie-break) and validates
// every id against mods, returning the resolved mod.Mod values in load
// order. Shared by resolveEnabledMods below (dlc_load.json's own
// descriptor-path strings) and writeClassicState (which also needs each
// mod's full data for its mods_registry.json entry, not just its id).
func resolveEnabledModList(order conflict.LoadOrder, mods []mod.Mod) ([]mod.Mod, error) {
	known := make(map[string]mod.Mod, len(mods))
	for _, m := range mods {
		known[m.ID] = m
	}

	seen := make(map[string]struct{}, len(order))
	var missing []string
	resolved := make([]mod.Mod, 0, len(order))
	for _, id := range order {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		m, ok := known[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		resolved = append(resolved, m)
	}

	if len(missing) > 0 {
		return nil, &UnknownModsError{IDs: missing}
	}
	return resolved, nil
}

// resolveEnabledMods is resolveEnabledModList, mapped down to just the
// descriptor-path strings dlc_load.json's enabled_mods field holds.
func resolveEnabledMods(order conflict.LoadOrder, mods []mod.Mod) ([]string, error) {
	resolved, err := resolveEnabledModList(order, mods)
	if err != nil {
		return nil, err
	}
	entries := make([]string, len(resolved))
	for i, m := range resolved {
		entries[i] = classicDescriptorPath(m.ID)
	}
	return entries, nil
}
