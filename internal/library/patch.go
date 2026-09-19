package library

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/patchmanifest"
)

// patchThumbnailName is the thumbnail's filename inside the patch mod, and
// what its descriptor's picture field points at. "thumbnail.png" is also the
// name Steam Workshop and this app's own ModThumbnail fall back to, so it's
// found even by tools that ignore the picture field.
const patchThumbnailName = "thumbnail.png"

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
	// Generation is which generation of the patch this was (1 for the
	// first), also the patch mod's own version.
	Generation int
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
//
// A conflict named in opts.Overrides is patched using that manually
// chosen mod instead of the automatic load-order winner (see
// effectiveWinner) - the exact same effective winner ConflictSummary.Winner
// already shows for that conflict, so what a user sees in the Conflict
// Resolver is always what actually gets patched.
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

	// Read what the previous patch recorded *before* the wipe below - it's
	// only there to keep the generation counter going.
	generation := 1
	if prev, ok := patchmanifest.Load(patchContentDir); ok {
		generation = prev.Generation + 1
	}

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

	// What this patch is being built from, for the manifest - see
	// internal/patchmanifest and patchstate.go.
	manifest := patchmanifest.Manifest{
		Generation:  generation,
		GeneratedAt: time.Now().Unix(),
		GameVersion: opts.GameVersion,
		Mods:        map[string]patchmanifest.ModRecord{},
		Keys:        make([]patchmanifest.KeyRecord, 0, len(rg.result.Conflicts)),
	}
	record := func(c conflict.Conflict, winner string, manual, wasSkipped bool) {
		sources := make(map[string]string, len(c.Candidates))
		for _, cand := range c.Candidates {
			sources[cand.ModID] = patchmanifest.Hash(cand.Hash)
			if m, ok := modsByID[cand.ModID]; ok {
				manifest.Mods[cand.ModID] = patchmanifest.ModRecord{Name: displayName(m), Version: m.Descriptor.Version}
			}
		}
		manifest.Keys = append(manifest.Keys, patchmanifest.KeyRecord{
			Type: string(c.Key.Type), ID: c.Key.ID,
			Winner: winner, Manual: manual, Skipped: wasSkipped,
			Sources: sources,
		})
	}

	for _, c := range rg.result.Conflicts {
		winner, manual := effectiveWinner(c, opts.Overrides)
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
			record(c, winner, manual, true)
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
			record(c, winner, manual, true)
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
		record(c, winner, manual, false)
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

	// The thumbnail goes in first so the descriptors below can name it only
	// if it really got written.
	picture := ""
	if len(opts.PatchThumbnail) > 0 {
		if _, err := atomicfile.Write(patchContentDir, patchThumbnailName, opts.PatchThumbnail); err != nil {
			return PatchResult{}, fmt.Errorf("library: writing patch thumbnail: %w", err)
		}
		picture = patchThumbnailName
	}

	desc := mod.Descriptor{
		Name:             "Parallax Mod Manager - Generated Patch",
		Version:          patchmanifest.VersionString(generation),
		SupportedVersion: supportedVersionPattern(opts.GameVersion),
		Picture:          picture,
		Tags:             []string{"Utilities", "Patch"},
		Dependencies:     patchDependencies(opts.Order, modsByID),
	}

	// A mod has two descriptors, and a proper one needs both: descriptor.mod
	// inside its own folder (what the game and the launcher read for the
	// mod's own metadata, and what makes the folder a self-contained mod
	// that can be moved or shared), and the stub in the game's mod folder
	// that registers it and says where it lives - the same two files every
	// mod the launcher or Steam installs has. The folder's copy has no path
	// (it's already where it says it is); the stub's is absolute.
	if _, err := atomicfile.Write(patchContentDir, "descriptor.mod", mod.WriteClassicDescriptor(desc)); err != nil {
		return PatchResult{}, fmt.Errorf("library: writing patch descriptor.mod: %w", err)
	}

	if err := patchmanifest.Write(patchContentDir, manifest); err != nil {
		return PatchResult{}, fmt.Errorf("library: %w", err)
	}

	// Registering the stub is last on purpose: until it exists the game
	// can't see the patch, so a failure anywhere above never leaves a
	// half-written patch loaded.
	stub := desc
	stub.Path = patchContentDir
	if _, err := atomicfile.Write(modDir, patchModID+".mod", mod.WriteClassicDescriptor(stub)); err != nil {
		return PatchResult{}, fmt.Errorf("library: writing patch descriptor: %w", err)
	}

	return PatchResult{Written: true, PatchedKeys: patched, SkippedKeys: skipped, ModID: patchModID, Generation: generation}, nil
}

// patchDependencies lists, by name and in load order, every mod that's loaded
// alongside the patch - the patch is built from all of them, so declaring them
// as its dependencies is what tells the launcher to load it after every one
// (and to warn when one is missing from a playset). Classic descriptors name a
// dependency by the mod's display name, so a mod that has no name of its own
// can't be listed, and one whose content is missing isn't loaded by the game
// anyway; each name appears once even if two mods share it. The patch itself
// is never its own dependency.
func patchDependencies(order conflict.LoadOrder, modsByID map[string]mod.Mod) []string {
	var names []string
	seen := map[string]bool{}
	for _, id := range order {
		if id == patchModID {
			continue
		}
		m, ok := modsByID[id]
		if !ok || m.ContentMissing || m.Descriptor.Name == "" || seen[m.Descriptor.Name] {
			continue
		}
		seen[m.Descriptor.Name] = true
		names = append(names, m.Descriptor.Name)
	}
	return names
}

// supportedVersionPattern turns a real game version ("v4.4.6") into the
// major.minor wildcard classic descriptors use ("v4.4.*"), which is what
// stops the launcher flagging the patch as made for a different game
// version. A version it can't split (empty, or no minor part) falls back to
// "*", meaning any version - never a guess.
func supportedVersionPattern(gameVersion string) string {
	v := strings.TrimSpace(gameVersion)
	if v == "" {
		return "*"
	}
	prefix := ""
	if v[0] == 'v' || v[0] == 'V' {
		prefix = "v"
		v = v[1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "*"
	}
	return prefix + parts[0] + "." + parts[1] + ".*"
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
