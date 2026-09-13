package library

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// localisationTypePrefix marks a definition.Type produced by
// definition.FromLocaleCatalog - "localisation/<language>", matching the
// real on-disk folder name Paradox games use (confirmed against a real
// Stellaris install). Unlike script Types, a localisation file needs a
// "l_<language>:" header line and a ".yml" extension to actually be read
// as a locale file - see writePatchContentFile.
const localisationTypePrefix = "localisation/"

// utf8BOM is prepended to generated .yml content, matching real Paradox
// locale files (confirmed on a real Stellaris install) - some non-English
// languages render incorrectly in-game without it.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// patchModID is the fixed identifier GeneratePatch always uses for its
// stub descriptor and content folder name. Prefixed with five z's -
// deliberately more aggressive than the community-typical "zzz_"
// convention - so its files sort alphabetically after essentially any
// real mod's when the game merges them: Stellaris processes files across
// every active mod (and the base game) in one merged, ASCIIbetical-by-
// filename order, falling back to mod load order only when two files
// share the exact same name. See docs/patch-mods.md.
const patchModID = "zzzzz_parallax_patch"

// PatchResult reports what GeneratePatch did.
type PatchResult struct {
	// Written is false when there were no patchable conflicts - nothing
	// was written, and any previous patch mod was removed (see
	// GeneratePatch's doc comment).
	Written bool
	// PatchedKeys counts genuine conflicts whose winning definition was
	// copied into the patch.
	PatchedKeys int
	// SkippedKeys counts genuine conflicts that couldn't be patched - a
	// winning definition with no extractable byte range, or whose source
	// file changed since that range was computed. See docs/patch-mods.md.
	SkippedKeys int
	// ModID is patchModID, handed back so callers don't need to know the
	// constant themselves.
	ModID string
}

// GeneratePatch resolves every genuine conflict in cfg's current mod set
// (per opts.Order) and writes a small, real mod - named to sort after
// everything else - that pins down each one's winning definition's exact
// original source bytes (via definition.Span, never a re-serialized parse
// tree, so the copied content is byte-for-byte what its author wrote).
//
// Always regenerates from a clean slate: any previous patch mod is deleted
// before rescanning. This matters for correctness, not just tidiness - a
// Type with no conflicts left over from a prior run must not keep
// overriding content nothing contests anymore, and leaving the old
// patch's own files in place while rescanning would let its stale content
// be mistaken for a genuine competing mod by this run's own conflict
// resolution, propagating it forward even after the mods it patched
// changed. See docs/patch-mods.md.
//
// Classic-descriptor games only - see docs/patch-mods.md for why this
// generates one patch per game, not per playset. Both classic script
// conflicts and localisation conflicts are patched (a localisation Type's
// content gets a real ".yml" file with its own language header line and
// UTF-8 BOM, not the plain ".txt" script files get - see
// patchContentFile).
func GeneratePatch(ctx context.Context, cfg game.GameConfig, opts Options) (PatchResult, error) {
	if cfg.DescriptorType != mod.DescriptorClassic {
		return PatchResult{}, fmt.Errorf("library: patch generation is only supported for classic-descriptor games, %s is not one", cfg.ID)
	}

	modDir := opts.ModDir
	if modDir == "" {
		userDir, err := cfg.UserDataDir()
		if err != nil {
			return PatchResult{}, fmt.Errorf("library: resolving user data dir for %s: %w", cfg.ID, err)
		}
		modDir = filepath.Join(userDir, "mod")
	}
	patchDescriptorPath := filepath.Join(modDir, patchModID+".mod")
	patchContentDir := filepath.Join(modDir, patchModID)

	if err := os.RemoveAll(patchDescriptorPath); err != nil {
		return PatchResult{}, fmt.Errorf("library: removing previous patch descriptor: %w", err)
	}
	if err := os.RemoveAll(patchContentDir); err != nil {
		return PatchResult{}, fmt.Errorf("library: removing previous patch content: %w", err)
	}

	rg, err := resolveConflicts(ctx, cfg, opts)
	if err != nil {
		return PatchResult{}, err
	}

	modsByID := make(map[string]mod.Mod, len(rg.mods))
	for _, m := range rg.mods {
		modsByID[m.ID] = m
	}

	type readKey struct{ modID, relPath string }
	fileCache := map[readKey][]byte{}
	readModFile := func(modID, relPath string) ([]byte, error) {
		k := readKey{modID, relPath}
		if data, ok := fileCache[k]; ok {
			return data, nil
		}
		m, ok := modsByID[modID]
		if !ok {
			return nil, fmt.Errorf("mod %q not found in current scan", modID)
		}
		data, err := os.ReadFile(filepath.Join(m.ContentPath, relPath))
		if err != nil {
			return nil, err
		}
		fileCache[k] = data
		return data, nil
	}

	byType := map[string]*bytes.Buffer{}
	var typeOrder []string
	patched, skipped := 0, 0

	for _, c := range rg.result.Conflicts {
		winner := winnerModID(c)
		var winnerDef *definition.Definition
		for i := range c.Candidates {
			if c.Candidates[i].ModID == winner {
				winnerDef = &c.Candidates[i]
				break
			}
		}
		// No extractable byte range - a Definition source that doesn't
		// populate Span.StartOffset/EndOffset (none currently exist, but
		// nothing here should assume that forever). Skip, don't guess.
		if winnerDef == nil || winnerDef.Span.EndOffset <= winnerDef.Span.StartOffset {
			skipped++
			continue
		}

		data, err := readModFile(winnerDef.ModID, winnerDef.FilePath)
		if err != nil {
			return PatchResult{}, fmt.Errorf("library: reading %s's %s for patch: %w", winnerDef.ModID, winnerDef.FilePath, err)
		}
		if winnerDef.Span.EndOffset > len(data) {
			// The file changed since this Span was computed - stale cache
			// entry or a concurrent edit. Skip rather than slice out of
			// bounds or copy the wrong bytes.
			skipped++
			continue
		}

		typeKey := string(c.Key.Type)
		buf, ok := byType[typeKey]
		if !ok {
			buf = &bytes.Buffer{}
			byType[typeKey] = buf
			typeOrder = append(typeOrder, typeKey)
		}
		buf.Write(data[winnerDef.Span.StartOffset:winnerDef.Span.EndOffset])
		buf.WriteString("\n\n")
		patched++
	}

	if patched == 0 {
		return PatchResult{Written: false, SkippedKeys: skipped}, nil
	}

	for _, typeKey := range typeOrder {
		dir := filepath.Join(patchContentDir, filepath.FromSlash(typeKey))
		filename, content := patchContentFile(typeKey, byType[typeKey].Bytes())
		if _, err := atomicfile.Write(dir, filename, content); err != nil {
			return PatchResult{}, fmt.Errorf("library: writing patch content for %s: %w", typeKey, err)
		}
	}

	desc := mod.Descriptor{
		Name:    "Parallax Mod Manager - Generated Patch",
		Path:    patchContentDir,
		Tags:    []string{"Utilities", "Patch"},
		Version: "1",
	}
	if _, err := atomicfile.Write(modDir, patchModID+".mod", mod.WriteClassicDescriptor(desc)); err != nil {
		return PatchResult{}, fmt.Errorf("library: writing patch descriptor: %w", err)
	}

	return PatchResult{Written: true, PatchedKeys: patched, SkippedKeys: skipped, ModID: patchModID}, nil
}

// patchContentFile returns the filename and final byte content for one
// Type's patch file. A localisation Type needs a real .yml locale file -
// its own "l_<language>:" header line (entries alone aren't a valid loc
// file) and the UTF-8 BOM real Paradox locale files carry - everything
// else is plain Clausewitz script, written as-is with a .txt extension.
func patchContentFile(typeKey string, entries []byte) (filename string, content []byte) {
	lang, ok := strings.CutPrefix(typeKey, localisationTypePrefix)
	if !ok {
		return patchModID + ".txt", entries
	}

	var buf bytes.Buffer
	buf.Write(utf8BOM)
	buf.WriteString("l_")
	buf.WriteString(lang)
	buf.WriteString(":\n")
	buf.Write(entries)
	return patchModID + ".yml", buf.Bytes()
}
