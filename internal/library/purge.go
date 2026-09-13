package library

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// EmptyModCandidate is one local mod with no real content to manage -
// either its declared content path doesn't exist at all, or it exists but
// contains no files - a candidate for the user to review and optionally
// delete via PurgeMods.
type EmptyModCandidate struct {
	ID     string
	Name   string
	Reason string // human-readable, e.g. "Content folder doesn't exist"
}

// FindEmptyMods finds every local mod whose content is missing or entirely
// empty - candidates for the Workspace's "Purge empty" action.
//
// Only Source local mods are ever considered: a Workshop item with no
// content yet is far more likely still downloading from Steam than
// genuinely broken, and its descriptor stub is Steam's or the Paradox
// Launcher's own file to manage, not this project's to delete out from
// under it.
func FindEmptyMods(ctx context.Context, cfg game.GameConfig, opts Options) ([]EmptyModCandidate, error) {
	scanResult, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: opts.SteamRoots, ModDir: opts.ModDir})
	if err != nil {
		return nil, fmt.Errorf("library: scanning %s: %w", cfg.ID, err)
	}

	// Starts as a real empty slice, not nil - the frontend's loading state
	// is "candidates === null" (a JS null, distinct from Go's own nil-slice
	// zero value); crossing this as JSON null would make "zero found" and
	// "still loading" indistinguishable on the other side.
	candidates := []EmptyModCandidate{}
	for _, m := range scanResult.Mods {
		if m.Source != mod.SourceLocal {
			continue
		}
		reason, empty := emptyReason(m)
		if !empty {
			continue
		}
		candidates = append(candidates, EmptyModCandidate{ID: m.ID, Name: displayName(m), Reason: reason})
	}
	return candidates, nil
}

// emptyReason reports whether m has no real content, and why.
func emptyReason(m mod.Mod) (reason string, empty bool) {
	if m.ContentMissing {
		return "Content folder doesn't exist", true
	}
	if dirHasNoFiles(m.ContentPath) {
		return "Content folder is empty", true
	}
	return "", false
}

// dirHasNoFiles reports whether root contains zero real files (folders
// alone don't count) - short-circuits on the first file found, unlike
// ListModFiles' full walk, since this only ever needs a yes/no answer. A
// root that can't be walked at all (doesn't exist, permission denied) also
// counts as having no files.
func dirHasNoFiles(root string) bool {
	hasFile := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			hasFile = true
			return filepath.SkipAll
		}
		return nil
	})
	return !hasFile
}

// PurgeResult reports what PurgeMods actually did.
type PurgeResult struct {
	// Deleted lists the mod IDs whose descriptor file was actually removed.
	Deleted []string
	// Errors collects one message per mod that couldn't be deleted, or
	// that no longer qualifies as empty - never fatal to the rest of the
	// batch, matching this project's non-fatal-per-item philosophy.
	Errors []string
}

// PurgeMods deletes the descriptor file for each mod in modIDs - and only
// the descriptor, never a mod's content folder (even an empty one), so
// this stays reversible in the one direction that matters: nothing but the
// small stub file cluttering the list is ever touched.
//
// Re-scans and re-verifies each mod still qualifies (Source local, and
// still missing or empty) right before deleting it, rather than trusting
// the caller's earlier FindEmptyMods snapshot blindly - closes a race where
// a mod's content reappeared (a drive reconnected, Workshop finished
// downloading) between when the candidate list was shown and confirmed.
func PurgeMods(ctx context.Context, cfg game.GameConfig, opts Options, modIDs []string) (PurgeResult, error) {
	scanResult, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: opts.SteamRoots, ModDir: opts.ModDir})
	if err != nil {
		return PurgeResult{}, fmt.Errorf("library: scanning %s: %w", cfg.ID, err)
	}
	byID := make(map[string]mod.Mod, len(scanResult.Mods))
	for _, m := range scanResult.Mods {
		byID[m.ID] = m
	}

	// Deleted/Errors start as real empty slices, not nil - crossing the JS
	// boundary as null (Go's nil-slice JSON encoding) where the frontend's
	// type says string[] would crash the first .length/.map call on it, the
	// same real bug already fixed once for ModSummary - see library.go's
	// package doc comment.
	result := PurgeResult{Deleted: []string{}, Errors: []string{}}
	for _, id := range modIDs {
		m, ok := byID[id]
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: no longer found", id))
			continue
		}
		if m.Source != mod.SourceLocal {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: not a local mod, skipped", displayName(m)))
			continue
		}
		if _, empty := emptyReason(m); !empty {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: has real content now, skipped", displayName(m)))
			continue
		}
		if err := os.Remove(m.DescriptorPath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", displayName(m), err))
			continue
		}
		result.Deleted = append(result.Deleted, id)
	}
	return result, nil
}
