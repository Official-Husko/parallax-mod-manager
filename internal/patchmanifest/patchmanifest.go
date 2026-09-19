// Package patchmanifest records what a generated patch mod was built from, so
// a later scan can tell when the patch has gone stale.
//
// A patch (see internal/library.GeneratePatch) pins one mod's version of every
// contested definition. That's only correct for as long as the mods it was
// built from still say what they said then: if a mod updates, another mod
// starts defining the same key, a source mod is removed, or the user changes
// who should win, the patch is silently out of date. The manifest keeps the
// evidence needed to notice - for every patched key, a content hash of every
// candidate's version - and lives inside the patch mod's own folder, so it
// is created and wiped together with the patch it describes and can never
// disagree with it about which patch it belongs to.
//
// Written as JSONC per CLAUDE.md: a header comment explains every field, so a
// person opening the file can tell what it is and why it's there.
package patchmanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// FileName is the manifest's name inside a patch mod's content folder.
const FileName = "parallax_patch_manifest.jsonc"

// FormatVersion guards the file's own shape. Bump it when a field's meaning
// changes; a file written by a newer version is not read (see Load).
const FormatVersion = 1

// HashVersion identifies how the per-candidate hashes were computed
// (currently: xxHash64 of internal/definition's whitespace-normalized
// definition text). Bump it whenever that normalization or hash function
// changes - every recorded hash would then differ from a freshly computed
// one for reasons that have nothing to do with a mod updating, and callers
// use a mismatch to say "regenerate" instead of flagging every key as
// individually changed.
const HashVersion = 1

// ModRecord is what the manifest remembers about one source mod, purely so a
// message can name it even after it's gone.
type ModRecord struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// KeyRecord is one contested definition the patch covers.
type KeyRecord struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	// Winner is the mod ID whose text the patch copied for this key.
	Winner string `json:"winner"`
	// Manual is true when Winner was a manual pick rather than the automatic
	// load-order winner.
	Manual bool `json:"manual,omitempty"`
	// Skipped is true when the key was contested but couldn't be written
	// into the patch (no extractable text, or the source file changed
	// mid-run). It's still recorded so it isn't reported as a new,
	// uncovered conflict on every scan.
	Skipped bool `json:"skipped,omitempty"`
	// Sources maps every candidate mod ID to the hash (see Hash) of its
	// version of this key at generation time.
	Sources map[string]string `json:"sources"`
}

// Manifest is the whole file.
type Manifest struct {
	FormatVersion int `json:"formatVersion"`
	HashVersion   int `json:"hashVersion"`
	// Generation counts how many times a patch has been generated for this
	// game (1 for the first). It also becomes the patch mod's own version.
	Generation int `json:"generation"`
	// GeneratedAt is when the patch was written, Unix seconds.
	GeneratedAt int64 `json:"generatedAt"`
	// GameVersion is the installed game's version at generation time, if it
	// was known.
	GameVersion string               `json:"gameVersion,omitempty"`
	Mods        map[string]ModRecord `json:"mods"`
	Keys        []KeyRecord          `json:"keys"`
}

// Hash renders a definition's uint64 content hash as a fixed-width hex
// string - a JSON number would lose precision in any JS-based tool that
// reads the file, and hex is what a person would want to eyeball anyway.
func Hash(h uint64) string {
	return fmt.Sprintf("%016x", h)
}

const header = `// Parallax Mod Manager - patch manifest.
//
// This file records what the generated patch mod next to it was built from, so the
// manager can tell when the patch has gone stale (a mod updated, another mod started
// defining the same key, a source mod was removed, or the chosen winner changed) and
// offer to regenerate it. It is rewritten every time the patch is generated and is
// deleted together with the patch. The game ignores it. Editing it by hand is safe
// but pointless: the next "Generate patch" replaces it.
//
// formatVersion   this file's own layout version.
// hashVersion     how the hashes below were computed; if it differs from what the
//                 manager uses now, every key is treated as needing a regenerate.
// generation      how many times a patch has been generated for this game.
// generatedAt     when this patch was written, in Unix seconds.
// gameVersion     the installed game's version at that moment, when known.
// mods            every mod the patch drew on or competed with, by mod ID, with the
//                 name and version it had then (so a message can name a mod that has
//                 since been removed).
// keys            one entry per contested definition the patch covers:
//   type, id      which definition (for example "common/buildings" and its tag).
//   winner        the mod ID whose text was copied into the patch for this key.
//   manual        true if that winner was a manual pick, not the load-order winner.
//   skipped       true if the key couldn't be written into the patch.
//   sources       mod ID -> hash of that mod's version of this key at generation
//                 time. A different hash later means that mod changed the definition.
`

// Write atomically writes m as JSONC into dir/FileName. Keys are written one
// per line: a real modlist can have thousands, and a pretty-printed entry
// per key would balloon the file for no benefit.
func Write(dir string, m Manifest) error {
	m.FormatVersion = FormatVersion
	m.HashVersion = HashVersion
	if m.Mods == nil {
		m.Mods = map[string]ModRecord{}
	}

	var b bytes.Buffer
	b.WriteString(header)
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"formatVersion\": %d,\n", m.FormatVersion)
	fmt.Fprintf(&b, "  \"hashVersion\": %d,\n", m.HashVersion)
	fmt.Fprintf(&b, "  \"generation\": %d,\n", m.Generation)
	fmt.Fprintf(&b, "  \"generatedAt\": %d,\n", m.GeneratedAt)
	if m.GameVersion != "" {
		gv, _ := json.Marshal(m.GameVersion)
		fmt.Fprintf(&b, "  \"gameVersion\": %s,\n", gv)
	}

	ids := make([]string, 0, len(m.Mods))
	for id := range m.Mods {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	b.WriteString("  \"mods\": {\n")
	for i, id := range ids {
		idJSON, _ := json.Marshal(id)
		recJSON, err := json.Marshal(m.Mods[id])
		if err != nil {
			return fmt.Errorf("patchmanifest: encoding mod %q: %w", id, err)
		}
		fmt.Fprintf(&b, "    %s: %s", idJSON, recJSON)
		if i < len(ids)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  },\n")

	b.WriteString("  \"keys\": [\n")
	for i, k := range m.Keys {
		if k.Sources == nil {
			k.Sources = map[string]string{}
		}
		kJSON, err := json.Marshal(k)
		if err != nil {
			return fmt.Errorf("patchmanifest: encoding key %s:%s: %w", k.Type, k.ID, err)
		}
		b.WriteString("    ")
		b.Write(kJSON)
		if i < len(m.Keys)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n}\n")

	if _, err := atomicfile.Write(dir, FileName, b.Bytes()); err != nil {
		return fmt.Errorf("patchmanifest: writing manifest: %w", err)
	}
	return nil
}

// Load reads dir/FileName. ok is false - never an error - when there's no
// manifest, it can't be parsed, or it was written by a newer format than this
// build understands: in every one of those cases the honest answer is "no
// usable record of what this patch was built from", which callers treat the
// same as "there is no patch to compare against".
func Load(dir string) (m Manifest, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return Manifest{}, false
	}
	if err := jsonc.Unmarshal(data, &m); err != nil {
		return Manifest{}, false
	}
	if m.FormatVersion <= 0 || m.FormatVersion > FormatVersion {
		return Manifest{}, false
	}
	if m.Mods == nil {
		m.Mods = map[string]ModRecord{}
	}
	return m, true
}

// VersionString is the patch mod's own version for a generation number
// ("1.3" for the third patch), so the launcher shows a version that moves
// every time the patch is regenerated.
func VersionString(generation int) string {
	return "1." + strconv.Itoa(generation)
}
