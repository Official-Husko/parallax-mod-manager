package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/pipeline"
	"github.com/Official-Husko/parallax-mod-manager/internal/translatecache"
	"github.com/Official-Husko/parallax-mod-manager/internal/xhash"
)

// SourceEntry is one English localisation key, ready to translate.
type SourceEntry struct {
	Key  string
	Text string
	// Hash is a hex xhash.Bytes of Text, the same identity translatecache
	// records to notice a later change - rendered here once so every
	// caller (Plan, and whoever persists a fresh translation) agrees on
	// the exact same string for the exact same text.
	Hash string
}

func hashText(text string) string {
	return fmt.Sprintf("%016x", xhash.Bytes([]byte(text)))
}

// localeFilesUnder returns modRoot's own real localisation/<folder>/*.yml
// files (root-relative, forward-slashed), in the same deterministic order
// pipeline.EnumerateFiles already guarantees - ASCIIbetical within the
// folder, so a later file's same-key entry is the one that wins on a
// same-mod clash, matching the last-file-wins convention already
// documented for cross-mod merging, applied here within one mod's own
// files instead.
//
// Note: this walks via cfg.ScanFolders, the same path every other feature
// in this app uses - which means it inherits that path's own known,
// pre-existing limitation (only Stellaris's ScanFolders list actually
// names "localisation" today; see the parent project's own plan notes) -
// not something this function tries to work around.
func localeFilesUnder(cfg game.GameConfig, modRoot, folder string) ([]string, error) {
	all, err := pipeline.EnumerateFiles(modRoot, cfg.ScanFolders)
	if err != nil {
		return nil, err
	}
	prefix := "localisation/" + folder + "/"
	var matched []string
	for _, rel := range all {
		slashRel := filepath.ToSlash(rel)
		if strings.HasPrefix(slashRel, prefix) && strings.HasSuffix(slashRel, ".yml") {
			matched = append(matched, rel)
		}
	}
	sort.Strings(matched)
	return matched, nil
}

// EnglishFileCount says how many localisation/english/*.yml files modRoot has - the Translate
// tab's own Source card wants a real file count alongside EnglishCatalog's own merged key count,
// and this is cheaper than re-deriving it from SourceEntry (which carries no per-file
// information at all, by design - see its own doc comment).
func EnglishFileCount(cfg game.GameConfig, modRoot string) (int, error) {
	files, err := localeFilesUnder(cfg, modRoot, "english")
	if err != nil {
		return 0, fmt.Errorf("library: finding %s's own English localisation files: %w", modRoot, err)
	}
	return len(files), nil
}

// EnglishCatalog reads every localisation/english/*.yml under modRoot and
// returns its merged key set, ready to translate. A key defined in more
// than one file within this same mod takes the later file's value
// (ASCIIbetical order), the same convention already established for
// cross-mod merging, applied here within one mod's own files instead.
func EnglishCatalog(cfg game.GameConfig, modRoot string) ([]SourceEntry, error) {
	files, err := localeFilesUnder(cfg, modRoot, "english")
	if err != nil {
		return nil, fmt.Errorf("library: finding %s's own English localisation files: %w", modRoot, err)
	}

	byKey := map[string]string{}
	var order []string
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(modRoot, rel))
		if err != nil {
			return nil, fmt.Errorf("library: reading %s: %w", rel, err)
		}
		cat, err := locale.Parse(data)
		if err != nil {
			continue // a genuinely unreadable file is skipped, not fatal to the whole mod - matches locale.Parse's own per-file tolerance elsewhere in this app
		}
		for _, e := range cat.Entries {
			if _, seen := byKey[e.Key]; !seen {
				order = append(order, e.Key)
			}
			byKey[e.Key] = e.Value
		}
	}

	sort.Strings(order)
	entries := make([]SourceEntry, 0, len(order))
	for _, key := range order {
		text := byKey[key]
		entries = append(entries, SourceEntry{Key: key, Text: text, Hash: hashText(text)})
	}
	return entries, nil
}

// TargetCatalog reads modRoot's own localisation/<folder>/*.yml the same
// way, key -> value - what already exists for one target language,
// whether from this feature's own earlier output or from anything else
// (the mod's own author, Steam, another tool).
func TargetCatalog(cfg game.GameConfig, modRoot, folder string) (map[string]string, error) {
	files, err := localeFilesUnder(cfg, modRoot, folder)
	if err != nil {
		return nil, fmt.Errorf("library: finding %s's own %s localisation files: %w", modRoot, folder, err)
	}
	result := map[string]string{}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(modRoot, rel))
		if err != nil {
			return nil, fmt.Errorf("library: reading %s: %w", rel, err)
		}
		cat, err := locale.Parse(data)
		if err != nil {
			continue
		}
		for _, e := range cat.Entries {
			result[e.Key] = e.Value
		}
	}
	return result, nil
}

// Need is one English key that still needs a real translate call for one
// target language.
type Need struct {
	Key         string
	EnglishText string
	EnglishHash string
	// Reason is "missing" (nothing at all exists yet) or "source-changed"
	// (a cached translation exists, but the English text it was made from
	// has since changed).
	Reason string
}

// Plan diffs english against what already exists (existingTarget) and
// what this feature already knows it translated before (cache, for this
// one language) - see the package's own translatecache.Record - to say
// exactly which keys still need a real API call, which already-covered
// keys are missing from the actual output file and just need rewriting
// from the cache (no API call), and how many keys needed nothing at all.
//
// A key with an existing target value but no cache record is
// pre-existing content this feature did not produce (a human, or another
// tool). It is never treated as a Need - it comes back in seeds instead,
// for the caller to record into the cache as translatecache.ServiceExisting
// (permanently exempt from ever being auto-re-translated - see that
// constant's own doc comment for why).
func Plan(english []SourceEntry, existingTarget map[string]string, cache translatecache.Record, lang string) (needs []Need, seeds []SourceEntry, needsRewriteFromCache []string, alreadyCovered int) {
	cached := cache.Entries[lang]

	for _, e := range english {
		entry, hasCache := cached[e.Key]
		_, hasExisting := existingTarget[e.Key]

		switch {
		case !hasCache && !hasExisting:
			needs = append(needs, Need{Key: e.Key, EnglishText: e.Text, EnglishHash: e.Hash, Reason: "missing"})
		case !hasCache && hasExisting:
			seeds = append(seeds, e)
		case hasCache && entry.SourceHash == e.Hash:
			alreadyCovered++
			if !hasExisting {
				needsRewriteFromCache = append(needsRewriteFromCache, e.Key)
			}
		case hasCache && entry.Service == translatecache.ServiceExisting:
			// Exempt: a pre-existing translation is never auto-refreshed
			// just because the English text moved on.
			alreadyCovered++
		default: // hasCache, hash mismatch, and this feature made the translation itself
			needs = append(needs, Need{Key: e.Key, EnglishText: e.Text, EnglishHash: e.Hash, Reason: "source-changed"})
		}
	}
	return needs, seeds, needsRewriteFromCache, alreadyCovered
}
