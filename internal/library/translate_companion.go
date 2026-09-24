package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// translationCompanionPrefix names every companion translation mod this
// feature ever generates - one per source mod, unlike the single, fixed
// zzzzz_parallax_patch. Deliberately an ordinary prefix, not five z's: a
// machine-translated fallback should never be forced to outrank a real,
// human-made translation mod defining the same key, the way the conflict
// patch mod deliberately is forced to win every tie it covers.
const translationCompanionPrefix = "parallax_translation_"

// translationCompanionManifestName is this companion's own small sidecar,
// living inside its content folder (that folder is fully app-owned and
// disposable, unlike a source mod's own real folder - see
// internal/translatecache's own doc comment on why the *per-key* cache
// still lives elsewhere regardless). Deliberately does not track per-key
// hashes - internal/translatecache already owns that; this just remembers
// which source mod this companion belongs to and how many times it has
// been touched.
const translationCompanionManifestName = "parallax_translation_manifest.jsonc"

// TranslationCompanionModID returns the fixed, deterministic mod ID
// sourceModID's own companion translation mod always uses.
func TranslationCompanionModID(sourceModID string) string {
	return translationCompanionPrefix + sanitizeForCompanionID(sourceModID)
}

func sanitizeForCompanionID(id string) string {
	return strings.NewReplacer("/", "_", `\`, "_", "..", "_").Replace(id)
}

// TranslationCompanionInfo is where one source mod's own companion
// translation mod lives on disk - resolved without creating anything (see
// EnsureTranslationCompanion for that).
type TranslationCompanionInfo struct {
	ModID      string
	ContentDir string
	StubPath   string
}

// ResolveTranslationCompanion works out sourceModID's companion's fixed
// paths under modDir (the launcher's own mod folder - the same modDir
// GeneratePatch itself resolves into).
func ResolveTranslationCompanion(modDir, sourceModID string) TranslationCompanionInfo {
	id := TranslationCompanionModID(sourceModID)
	return TranslationCompanionInfo{
		ModID:      id,
		ContentDir: filepath.Join(modDir, id),
		StubPath:   filepath.Join(modDir, id+".mod"),
	}
}

type translationCompanionManifest struct {
	FormatVersion   int    `json:"formatVersion"`
	SourceModID     string `json:"sourceModId"`
	Generation      int    `json:"generation"`
	LastGeneratedAt int64  `json:"lastGeneratedAt"`
}

const translationCompanionManifestHeader = `// Parallax Mod Manager - auto-translation companion mod.
//
// This mod holds machine-translated localisation for another mod (sourceModId
// below), generated so its content can be read without ever writing into that
// other mod's own real files - safe to use even for a Steam Workshop or Paradox
// Launcher mod. Its own actual translated .yml files are added to or updated in
// place as you translate more languages or more keys - never wiped and rebuilt
// from scratch, unlike this app's own generated conflict patch mod, since
// re-translating unchanged text would waste real translation-service quota.
//
// formatVersion    this file's own layout version.
// sourceModId      the mod this companion's translations are for.
// generation       how many times this companion has been touched.
// lastGeneratedAt  when it was last touched, Unix seconds.
`

// EnsureTranslationCompanion creates info's own descriptor/stub/manifest if
// they don't exist yet, or refreshes the manifest's generation/timestamp
// if they do - called every time a player-mode translate run writes
// anything, so a person can always tell (via the manifest) that this
// companion is current, even though the actual locale content files
// themselves are never wiped wholesale the way GeneratePatch's own patch
// mod is. sourceModName is the source mod's own display name, used for the
// companion's own Name and Dependencies (the same "name a real dependency
// by its display name" convention patchDependencies already uses).
func EnsureTranslationCompanion(info TranslationCompanionInfo, sourceModID, sourceModName, gameVersion string) error {
	generation := 1
	if prevData, err := os.ReadFile(filepath.Join(info.ContentDir, translationCompanionManifestName)); err == nil {
		var prev translationCompanionManifest
		if jsonErr := jsonc.Unmarshal(prevData, &prev); jsonErr == nil && prev.Generation > 0 {
			generation = prev.Generation + 1
		}
	}

	desc := mod.Descriptor{
		Name:             "Parallax Auto-Translations: " + sourceModName,
		Version:          fmt.Sprintf("1.%d", generation),
		SupportedVersion: supportedVersionPattern(gameVersion),
		Tags:             []string{"Utilities", "Translation"},
		Dependencies:     []string{sourceModName},
	}
	if _, err := atomicfile.Write(info.ContentDir, "descriptor.mod", mod.WriteClassicDescriptor(desc)); err != nil {
		return fmt.Errorf("library: writing translation companion descriptor for %s: %w", sourceModID, err)
	}

	manifest := translationCompanionManifest{
		FormatVersion:   1,
		SourceModID:     sourceModID,
		Generation:      generation,
		LastGeneratedAt: time.Now().Unix(),
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("library: encoding translation companion manifest for %s: %w", sourceModID, err)
	}
	manifestBody := append([]byte(translationCompanionManifestHeader), manifestJSON...)
	manifestBody = append(manifestBody, '\n')
	if _, err := atomicfile.Write(info.ContentDir, translationCompanionManifestName, manifestBody); err != nil {
		return fmt.Errorf("library: writing translation companion manifest for %s: %w", sourceModID, err)
	}

	// Registering the stub last, same reason GeneratePatch does: until it
	// exists the game/launcher can't see this companion at all, so a
	// failure above never leaves a half-written one registered.
	stub := desc
	stub.Path = info.ContentDir
	if _, err := atomicfile.Write(filepath.Dir(info.StubPath), filepath.Base(info.StubPath), mod.WriteClassicDescriptor(stub)); err != nil {
		return fmt.Errorf("library: writing translation companion stub for %s: %w", sourceModID, err)
	}
	return nil
}
