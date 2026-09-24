// Package translatecache is the auto-translation feature's own incremental
// cache: which (target language, localisation key) pairs have already
// been translated for a given mod, and from what English text - so
// re-running the translator only pays for what is new or changed, never
// wiping and redoing everything the way internal/library.GeneratePatch's
// own patch mod deliberately does (a real, deliberate divergence from that
// precedent: translation API calls have real cost/quota, unlike free
// local disk I/O).
//
// Stored at <configAppDir>/translate_cache/<gameID>/<safeModID>.jsonc -
// never inside any mod's own folder, for both of the feature's two
// destination modes uniformly:
//
//   - "Author mode" (writing straight into a real mod the person owns)
//     cannot put bookkeeping inside that mod's own folder at all - this
//     app's own established rule ("every save keeps the files it replaced
//     in the settings folder, never inside the mod, which is what gets
//     uploaded") rules that out outright.
//   - "Player mode"'s companion-mod folder could technically host an
//     in-folder manifest, the way patchmanifest does for the patch mod -
//     but that folder is disposable (a person can delete it, or this app's
//     own regeneration could recreate it), and if the cache lived only
//     there, losing it would silently erase the record of API quota
//     already spent, forcing a costly, unnecessary full re-translation.
//     Keeping the cache outside the disposable output protects that cost.
//
// This makes the cache authoritative and on-disk output (a source mod's
// own localisation/<lang>/ folder, or a companion mod's content folder) a
// regenerable projection of it, in both modes - never the other way
// around.
package translatecache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// FormatVersion guards the file's own shape - a file written by a newer
// version is not read (see Load), the same rule internal/patchmanifest's
// own FormatVersion follows.
const FormatVersion = 1

// Entry is what's remembered about one already-translated key, for one
// target language.
type Entry struct {
	// SourceHash is a hex xhash.Bytes of the English text this entry was
	// translated from, at translation time - a later mismatch means the
	// English text has since changed, and (unless Service is "existing")
	// this entry needs re-translating.
	SourceHash string `json:"sourceHash"`
	// TranslatedText is the actual translated value, rendered straight
	// into the output file by internal/locale.RenderFile without another
	// API call.
	TranslatedText string `json:"translatedText"`
	// Service is which of "deepl", "translanova", "vust" produced this
	// entry, or "existing" for a value this feature found already present
	// (a human, or another tool, translated it before this ever ran) -
	// see ServiceExisting's own doc comment for what that means for
	// re-translation.
	Service string `json:"service"`
	// TranslatedAt is when this entry was recorded, Unix seconds.
	TranslatedAt int64 `json:"translatedAt"`
}

// ServiceExisting marks an Entry seeded from a pre-existing target-language
// value this feature did not itself produce (found already present with no
// cache record) - permanently exempt from ever being auto-re-translated
// later, even once its English source text changes, so a real person's (or
// another tool's) translation is never silently overwritten just because
// the English line was edited afterwards.
const ServiceExisting = "existing"

// Record is one mod's whole cache, across every target language it has
// ever been translated into.
type Record struct {
	FormatVersion int `json:"formatVersion"`
	// Entries maps a target language's DeepL code to that language's own
	// key -> Entry map. A Go map serializes with sorted keys automatically
	// (encoding/json's own documented behavior), so this file's own
	// on-disk key order is already stable and diffable without needing
	// patchmanifest's own hand-rolled, manually-sorted writer.
	Entries map[string]map[string]Entry `json:"entries"`
}

const header = `// Parallax Mod Manager - auto-translation cache.
//
// Remembers which localisation keys have already been machine-translated for this
// mod, for which target languages, and from what English text - so re-running the
// translator only pays for what is new or changed. Never wiped and rebuilt from
// scratch the way the generated patch mod's own manifest is: losing this file would
// mean silently re-spending real translation-service quota re-translating text that
// never changed. Safe to delete by hand if you want to force everything to be
// re-translated from scratch, but there is normally no reason to.
//
// formatVersion   this file's own layout version.
// entries         target language code -> localisation key -> what was translated:
//   sourceHash      a hash of the English text this was translated from - a later
//                   mismatch means that English text has since changed.
//   translatedText  the actual translated value.
//   service         which service produced this ("deepl", "translanova", "vust"),
//                   or "existing" for a value this feature found already present
//                   and never touches again, even once the English text changes.
//   translatedAt    when this was recorded, Unix seconds.
`

// safeModID mirrors internal/app/modedit.go's own mod-ID sanitizer exactly
// (kept as a private copy here rather than an import, the same reason
// companions/launcher-shim mirrors rather than imports its own status
// type - this package has no reason to depend on internal/app).
func safeModID(modID string) string {
	return strings.NewReplacer("/", "_", `\`, "_", "..", "_").Replace(modID)
}

func path(configDir, gameID, modID string) string {
	return filepath.Join(configDir, "translate_cache", gameID, safeModID(modID)+".jsonc")
}

// Load returns modID's saved cache. ok is false - never an error - when
// there is no file yet, it cannot be parsed, or it was written by a newer
// format than this build understands - in every one of those cases the
// honest answer is "no usable cache", which callers treat as "nothing has
// been translated yet", the same tolerance internal/patchmanifest.Load
// already documents.
func Load(configDir, gameID, modID string) (Record, bool) {
	data, err := os.ReadFile(path(configDir, gameID, modID))
	if err != nil {
		return Record{}, false
	}
	var rec Record
	if err := jsonc.Unmarshal(data, &rec); err != nil {
		return Record{}, false
	}
	if rec.FormatVersion <= 0 || rec.FormatVersion > FormatVersion {
		return Record{}, false
	}
	if rec.Entries == nil {
		rec.Entries = map[string]map[string]Entry{}
	}
	return rec, true
}

// Save atomically writes rec as modID's whole cache, replacing whatever
// was there before - callers always pass the full, merged Record (see
// internal/library/translate_plan.go), never a partial diff, since this
// package has no merge logic of its own.
func Save(configDir, gameID, modID string, rec Record) error {
	if configDir == "" {
		return fmt.Errorf("translatecache: no config directory to save into")
	}
	rec.FormatVersion = FormatVersion
	if rec.Entries == nil {
		rec.Entries = map[string]map[string]Entry{}
	}

	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("translatecache: encoding cache for %q: %w", modID, err)
	}
	full := make([]byte, 0, len(header)+len(body)+1)
	full = append(full, header...)
	full = append(full, body...)
	full = append(full, '\n')

	p := path(configDir, gameID, modID)
	if _, err := atomicfile.Write(filepath.Dir(p), filepath.Base(p), full); err != nil {
		return fmt.Errorf("translatecache: saving cache for %q: %w", modID, err)
	}
	return nil
}
