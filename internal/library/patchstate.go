package library

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/patchmanifest"
	"github.com/Official-Husko/parallax-mod-manager/internal/patchoverride"
)

// ConflictSummary.PatchState values. "" means there is no generated patch to
// compare against at all, so no conflict carries a state.
const (
	// PatchStatePatched: the patch covers this key and nothing it was built
	// from has changed since.
	PatchStatePatched = "patched"
	// PatchStateChanged: the patch covers this key, but something it was
	// built from has changed - a source mod updated, another mod now also
	// defines the key, a source mod is gone, or the winner is no longer the
	// one the patch pinned. The patch's copy of this key can't be trusted
	// until it's reviewed and the patch regenerated.
	PatchStateChanged = "changed"
	// PatchStateNew: a patch exists, but it was generated before this key
	// became a conflict, so it isn't covered.
	PatchStateNew = "new"
)

// PatchSummary describes the generated patch as of this scan, for the
// frontend's "your patch is out of date" prompt. The zero value (Exists
// false) means no patch has been generated for this game, or its manifest
// couldn't be read.
type PatchSummary struct {
	Exists bool
	// GeneratedAt is when the patch was last written, Unix seconds - safe to
	// cross the Wails/JS boundary as a plain number, same reasoning as
	// ModFiles.LastModified.
	GeneratedAt int64
	// Generation counts how many times a patch has been generated (1 = the
	// first).
	Generation int
	// Patched counts conflicts the patch covers whose sources are unchanged.
	Patched int
	// Changed counts conflicts the patch covers whose sources changed.
	Changed int
	// New counts current conflicts the patch doesn't cover at all.
	New int
	// Obsolete counts keys the patch covers that are no longer conflicts
	// (the mods involved changed, were removed, or were disabled). They stay
	// in the patch harmlessly until it's regenerated.
	Obsolete int
	// GeneratedForVersion is the game version the patch was made for, as it
	// was recorded ("" for a patch made before that was tracked, or when the
	// version was unknown).
	GeneratedForVersion string
	// GameVersion is the version the game reports now, when known.
	GameVersion string
	// GameChanged is true when the game has moved to a different major.minor
	// (v4.4.x to v4.5.x) since the patch was made. The patch declares itself
	// compatible with the version it was made for (supported_version), so after
	// such an update the launcher flags it as made for another game version,
	// and the mods it was built from may have been updated to match. A change
	// within a minor version (v4.4.5 to v4.4.6) doesn't count: the patch's
	// wildcard still covers it.
	GameChanged bool
	// ChangedMods names every mod whose content for a covered key changed,
	// was added to it, or is gone - what a "check these mods" prompt should
	// point at. Never nil; sorted.
	ChangedMods []string
}

// NeedsAttention reports whether anything about the patch is worth prompting
// the user over: covered keys whose sources changed, current conflicts it
// doesn't cover, or keys it covers that no longer conflict. A patch that's
// only ever been made stale by keys becoming obsolete is the mildest case,
// but it's still out of date.
func (p PatchSummary) NeedsAttention() bool {
	return p.Exists && (p.Changed > 0 || p.New > 0 || p.Obsolete > 0 || p.GameChanged)
}

// patchContentDir returns where cfg's generated patch mod keeps its content
// (and its manifest), using the same mod-folder resolution GeneratePatch
// does. ok is false if the mod folder can't be resolved.
func patchContentDir(cfg game.GameConfig, opts Options) (dir string, ok bool) {
	modDir := opts.ModDir
	if modDir == "" {
		userDir, err := cfg.UserDataDir()
		if err != nil {
			return "", false
		}
		modDir = filepath.Join(userDir, "mod")
	}
	return filepath.Join(modDir, patchModID), true
}

// describePatch is what is known about the generated patch without comparing
// it to anything: that it exists, when and for which game version it was made,
// and whether the game has moved on since. Every count is zero - which is also
// the answer when there is nothing to compare against, see LoadGame.
func describePatch(manifest patchmanifest.Manifest, gameVersion string) PatchSummary {
	out := PatchSummary{
		Exists:              true,
		GeneratedAt:         manifest.GeneratedAt,
		Generation:          manifest.Generation,
		GeneratedForVersion: manifest.GameVersion,
		GameVersion:         gameVersion,
		ChangedMods:         []string{},
	}
	// Only when both versions are known and read as a real major.minor: an
	// older patch that never recorded one, or a game that reports none, is
	// unknown, not changed.
	if was, now := supportedVersionPattern(manifest.GameVersion), supportedVersionPattern(gameVersion); was != "*" && now != "*" {
		out.GameChanged = was != now
	}
	return out
}

// applyPatchState compares the current conflicts against manifest - what the
// generated patch was built from - and records the outcome on summaries (one
// per conflict, same order) and in the returned PatchSummary.
//
// The comparison is on content, not on file timestamps or mod versions: a
// mod update that never touched a key the patch covers changes nothing here,
// and a mod that silently rewrites one definition without bumping its
// version is still caught.
func applyPatchState(summaries []ConflictSummary, conflicts []conflict.Conflict, manifest patchmanifest.Manifest, names map[string]string, gameVersion string) PatchSummary {
	out := describePatch(manifest, gameVersion)

	covered := make(map[string]patchmanifest.KeyRecord, len(manifest.Keys))
	for _, k := range manifest.Keys {
		covered[patchoverride.Key(k.Type, k.ID)] = k
	}

	nameOf := func(id string) string {
		if n, ok := names[id]; ok && n != "" {
			return n
		}
		if r, ok := manifest.Mods[id]; ok && r.Name != "" {
			return r.Name
		}
		return id
	}

	// Every recorded hash is meaningless if it was computed differently to how
	// this build computes them - say so once per key instead of flagging
	// unrelated "changes".
	hashesComparable := manifest.HashVersion == patchmanifest.HashVersion

	touched := map[string]bool{} // mod IDs behind at least one change
	seen := make(map[string]bool, len(conflicts))

	for i, c := range conflicts {
		key := patchoverride.Key(string(c.Key.Type), c.Key.ID)
		rec, isCovered := covered[key]
		if !isCovered {
			summaries[i].PatchState = PatchStateNew
			summaries[i].PatchNote = "Not in the generated patch - this conflict appeared after it was made."
			out.New++
			continue
		}
		seen[key] = true

		var reasons []string
		if !hashesComparable {
			reasons = append(reasons, "The patch was made by an older version of the manager and can't be compared.")
		} else {
			now := make(map[string]string, len(c.Candidates))
			for _, cand := range c.Candidates {
				now[cand.ModID] = patchmanifest.Hash(cand.Hash)
			}
			var updated, added, removed []string
			for _, cand := range c.Candidates {
				was, wasThere := rec.Sources[cand.ModID]
				switch {
				case !wasThere:
					added = append(added, nameOf(cand.ModID))
					touched[cand.ModID] = true
				case was != now[cand.ModID]:
					updated = append(updated, nameOf(cand.ModID))
					touched[cand.ModID] = true
				}
			}
			for id := range rec.Sources {
				if _, still := now[id]; !still {
					removed = append(removed, nameOf(id))
					touched[id] = true
				}
			}
			sort.Strings(removed)
			if len(updated) > 0 {
				reasons = append(reasons, "Changed since the patch: "+strings.Join(updated, ", ")+".")
			}
			if len(added) > 0 {
				reasons = append(reasons, "Now also defined by: "+strings.Join(added, ", ")+".")
			}
			if len(removed) > 0 {
				reasons = append(reasons, "No longer loaded: "+strings.Join(removed, ", ")+".")
			}
		}
		if summaries[i].Winner != rec.Winner {
			reasons = append(reasons, "The winner is now "+nameOf(summaries[i].Winner)+", but the patch pins "+nameOf(rec.Winner)+".")
		}

		if len(reasons) == 0 {
			summaries[i].PatchState = PatchStatePatched
			out.Patched++
			continue
		}
		summaries[i].PatchState = PatchStateChanged
		summaries[i].PatchNote = strings.Join(reasons, " ")
		out.Changed++
	}

	out.Obsolete = len(covered) - len(seen)

	for id := range touched {
		out.ChangedMods = append(out.ChangedMods, nameOf(id))
	}
	sort.Strings(out.ChangedMods)
	return out
}
