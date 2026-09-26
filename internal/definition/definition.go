// Package definition maps parsed script/localization content onto the
// vocabulary conflict-resolution needs: one Definition per top-level
// object, identified by (Type, ID), content-hashed for comparison. See
// docs/conflict-resolution.md.
//
// The conflict resolver itself (indexing by (Type,ID), grouping by hash,
// LIOS/FIOS priority) is out of scope here - this package only produces the
// Definitions it will need.
package definition

import (
	"bytes"
	"path"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
	"github.com/Official-Husko/parallax-mod-manager/internal/xhash"
)

// Type is a content category, derived from the mod-relative folder a file
// lives in (e.g. "common/buildings", "events", "localisation/english").
type Type string

// Span locates a definition's source text for on-demand re-read (a diff
// view, a manual conflict resolution) without keeping the text itself
// resident in memory - see docs/performance-strategy.md point 4.
type Span struct {
	StartOffset, EndOffset int
	StartLine, EndLine     int
}

// Definition is one top-level object extracted from a mod file: one
// country_event block, one building, one localization entry, etc.
type Definition struct {
	Type     Type
	ID       string
	ModID    string
	FilePath string // mod-relative
	Hash     uint64 // xhash.Definition of Normalize(value) - identity for grouping
	Span     Span
	Order    int // position within the file; a future FIOS tie-break needs this
}

// eventsType is Stellaris' own events Type - see NestedIDFields and
// skipTopLevelKeys, both of which have a confirmed entry for it.
const eventsType = Type("events")

// NestedIDFields names, for a specific Type, the nested field within each
// top-level entry's own block that carries its real, unique identifier -
// for Types where the top-level key itself is just a shared category
// keyword, never a usable id on its own. A Type absent from this map keeps
// the default: its own top-level key is the id (correct for the common
// case, e.g. common/buildings, where the top-level key is the building's
// own tag).
//
// Confirmed so far:
//   - Stellaris' "events": every top-level entry is one of a handful of
//     generic category keywords ("country_event", "ship_event",
//     "fleet_event", ...) shared by thousands of unrelated events -
//     confirmed against a real install, "country_event" alone is the
//     literal top-level key of 5,000+ distinct vanilla events. The real,
//     globally unique identifier is nested as "id = <namespace>.<number>".
//     This convention is a long-standing, near-universal Clausewitz one,
//     confirmed here against Stellaris specifically and assumed, not
//     independently reverified, to hold for every other Paradox game's
//     own identically-named "events" folder.
//   - Stellaris' "common/section_templates": every top-level entry is the
//     single literal keyword "ship_section_template" - confirmed against
//     a real install, hundreds of distinct vanilla section templates all
//     share it. The real, unique name is nested as "key = "SOME_NAME"".
//     Paradox's own wiki confirms a genuine duplicate here is destructive
//     ("existing can't be overwritten, used ships/starbases will get
//     deleted") - all the more reason getting the real identity right
//     matters: without it, every section template in every mod looked
//     like it was colliding with every other one, a false positive on
//     the same scale as the events case above.
//   - Stellaris' "common/message_types": every top-level entry is the
//     single literal keyword "message_type" - the real vanilla file
//     (common/message_types/00_message_types.txt) is entirely commented
//     out (it ships zero real message types itself, purely as a
//     documentation template for modders), but confirms the same wrapper
//     shape: the real, unique name is nested as "key = "SOME_NAME"".
//     Found by the same method as section_templates above: a real
//     conflict naming five unrelated mods as if they shared one object.
//   - Stellaris' "gfx/projectiles": every top-level entry is one of a
//     handful of category keywords ("projectile_gfx_beam",
//     "projectile_gfx_ballistic", "projectile_gfx_missile") shared across
//     18 distinct vanilla files - confirmed the real, unique name is
//     nested as "name = "ion_cannon"" (unlike the .gfx-extension case
//     these files structurally resemble, this folder's own files are
//     plain ".txt" despite living under "gfx/", so the file-extension-
//     based fromWrappedObjectTypes below never applies here - this Type
//     needed its own entry instead).
//   - Stellaris' "gfx/worldgfx": every top-level entry is the single
//     literal keyword "gfx_settings", confirmed repeated across 21
//     distinct vanilla files; the real, unique name is nested as
//     "world = customization_view_planet".
//   - Stellaris' "common/ambient_objects": every top-level entry is the
//     single literal keyword "ambient_object", confirmed repeated 220+
//     times across more than 20 distinct vanilla files; the real, unique
//     name is nested as "name = "habitat_cracker_object"".
var NestedIDFields = map[Type]string{
	eventsType:                       "id",
	Type("common/section_templates"): "key",
	Type("common/message_types"):     "key",
	Type("gfx/projectiles"):          "name",
	Type("gfx/worldgfx"):             "world",
	Type("common/ambient_objects"):   "name",
}

// wholeFileTypePrefix is the folder (and every one of its own subfolders)
// where an entire *file* - not any of its own top-level entries - is the
// real conflictable unit, and the file's own name (without extension) is
// its real identity, not anything inside it. Confirmed directly from the
// game's own documentation, common/inline_scripts/00_README.txt: "you can
// only have one inline script per file and... the file name will be used
// when inlining the script into other scripts" (elsewhere referenced via
// `inline_script = "traits/radiotrophic_effects"`, a path relative to this
// folder, extension omitted) - "you can organize the inline scripts by
// putting them inside subfolders" confirms subfolders share the same
// convention, not just the top-level folder itself.
//
// Before this was recognized, every top-level *field* inside one of these
// files (icon = ..., resources = {...}, modifier = {...} - these files are
// literally just a flat list of fields, no wrapper key at all) was wrongly
// treated as its own separate definition, competing with any other mod's
// completely unrelated inline-script file that happened to define a field
// with the same name - confirmed on a real install: a real conflict named
// two entirely unrelated mods' files as if they shared an object, just
// because both happened to define a field called "resources" or "icon".
const wholeFileTypePrefix = "common/inline_scripts"

// IsWholeFileType reports whether t is wholeFileTypePrefix or one of its
// own subfolders.
func IsWholeFileType(t Type) bool {
	return string(t) == wholeFileTypePrefix || strings.HasPrefix(string(t), wholeFileTypePrefix+"/")
}

// skipTopLevelKeys names, for a specific Type, top-level keys that should
// never become a Definition at all - directives with no content of their
// own to conflict over, distinct from a NestedIDFields entry (which still
// produces a Definition, just under a different id).
//
// Confirmed so far:
//   - Stellaris' "events", key "namespace": the bare directive
//     ("namespace = foo") every real events file starts with, establishing
//     the file's own id prefix. Its literal key repeats across virtually
//     every mod's own events files (160+ times in the vanilla install's
//     own events/ folder alone) - treating it as a definition would flag a
//     bogus conflict between any two mods that each declare one, the same
//     problem NestedIDFields solves for actual events.
var skipTopLevelKeys = map[Type]map[string]bool{
	eventsType: {"namespace": true},
}

// gfxGUIExtensions names the file extensions whose real per-object identity
// lives one level deeper than an ordinary script file's top-level entries -
// confirmed against a real Stellaris install: every ".gfx" file wraps its
// actual content in a single, always-present "objectTypes = { ... }" block,
// and every ".gui" file the same way in "guiTypes = { ... }". The wrapper
// itself is never meaningful - there's exactly one per file, the same
// literal key every time, never a real identity to conflict over - and
// each of *its own* children (the specific keyword varies by content -
// "pdxparticle" for particles, "pdxmesh" for models, "containerWindowType"
// for a gui window, ... - but every one of them carries its own real
// "name" field) is the actual conflictable object. Gated by file
// extension, not by Type/folder: this convention is universal across
// every .gfx/.gui file regardless of which folder it lives in, unlike
// NestedIDFields/skipTopLevelKeys/IsWholeFileType above, which are each
// specific to one particular folder.
var gfxGUIExtensions = map[string]bool{".gfx": true, ".gui": true}

// FromScriptFile extracts one Definition per top-level entry of a parsed
// Clausewitz script file. ID defaults to the entry's own key; defType's
// presence in NestedIDFields/skipTopLevelKeys is a confirmed exception -
// see their own doc comments. IsWholeFileType(defType) and
// gfxGUIExtensions are two different kinds of exception entirely - see
// fromWholeFile and fromWrappedObjectTypes respectively.
func FromScriptFile(modID, relPath string, defType Type, f *script.File) []Definition {
	if f == nil {
		return nil
	}
	if IsWholeFileType(defType) {
		return fromWholeFile(modID, relPath, defType, f)
	}
	if ext := path.Ext(strings.ReplaceAll(relPath, `\`, "/")); gfxGUIExtensions[strings.ToLower(ext)] {
		return fromWrappedObjectTypes(modID, relPath, defType, f)
	}
	nestedField := NestedIDFields[defType]
	skip := skipTopLevelKeys[defType]
	defs := make([]Definition, 0, len(f.Root.Entries))
	for i, e := range f.Root.Entries {
		if e.Key == "" {
			// A bare list item at the top level of a file has no natural id
			// and isn't a conflictable "object" on its own; skip it.
			continue
		}
		if skip[e.Key] {
			continue
		}
		id := e.Key
		if nestedField != "" {
			if nested, ok := nestedFieldRaw(e.Value, nestedField); ok {
				id = nested
			}
		}
		defs = append(defs, Definition{
			Type:     defType,
			ID:       id,
			ModID:    modID,
			FilePath: relPath,
			Hash:     xhash.Definition(Normalize(e.Value)),
			Span: Span{
				StartOffset: e.Offset,
				EndOffset:   e.EndOffset,
				StartLine:   e.Pos.Line,
				EndLine:     e.Value.Pos.Line,
			},
			Order: i,
		})
	}
	return defs
}

// fromWrappedObjectTypes extracts one Definition per child of a .gfx/.gui
// file's own top-level wrapper block (see gfxGUIExtensions) - the wrapper
// itself is skipped, never a Definition of its own, since it's the same
// single literal key in every file of its kind and never itself a usable
// id. A child's id is its own nested "name" field when present (the
// confirmed, universal convention); a child without one keeps its own key
// instead, a defensive fallback for anything that doesn't follow it, not a
// confirmed exception. A top-level entry that isn't a block at all (not
// the expected wrapper shape) is kept as its own Definition rather than
// silently dropped, the same defensive spirit.
func fromWrappedObjectTypes(modID, relPath string, defType Type, f *script.File) []Definition {
	var defs []Definition
	order := 0
	appendDef := func(e script.Entry, id string) {
		defs = append(defs, Definition{
			Type:     defType,
			ID:       id,
			ModID:    modID,
			FilePath: relPath,
			Hash:     xhash.Definition(Normalize(e.Value)),
			Span: Span{
				StartOffset: e.Offset,
				EndOffset:   e.EndOffset,
				StartLine:   e.Pos.Line,
				EndLine:     e.Value.Pos.Line,
			},
			Order: order,
		})
		order++
	}
	for _, wrapper := range f.Root.Entries {
		if wrapper.Value.Kind != script.KindBlock || wrapper.Value.Block == nil {
			appendDef(wrapper, wrapper.Key)
			continue
		}
		for _, child := range wrapper.Value.Block.Entries {
			if child.Key == "" {
				continue
			}
			id := child.Key
			if nested, ok := nestedFieldRaw(child.Value, "name"); ok {
				id = nested
			}
			appendDef(child, id)
		}
	}
	return defs
}

// fromWholeFile builds the single Definition a whole-file Type (see
// IsWholeFileType) gets for one file: id is the file's own name without
// extension - the real identity every reference to it uses - content is
// the entire file's top-level block, normalized and hashed as one unit
// (reusing Normalize/normalizeValue's own existing block-handling rather
// than duplicating it), and Span covers from its first entry's own start
// to its last entry's own end. Empty (no entries at all - a blank or
// fully-commented file) yields no Definition; there's nothing to conflict
// over.
func fromWholeFile(modID, relPath string, defType Type, f *script.File) []Definition {
	if len(f.Root.Entries) == 0 {
		return nil
	}
	base := path.Base(strings.ReplaceAll(relPath, `\`, "/"))
	id := strings.TrimSuffix(base, path.Ext(base))
	first, last := f.Root.Entries[0], f.Root.Entries[len(f.Root.Entries)-1]
	return []Definition{{
		Type:     defType,
		ID:       id,
		ModID:    modID,
		FilePath: relPath,
		Hash:     xhash.Definition(Normalize(script.Value{Kind: script.KindBlock, Block: &f.Root})),
		Span: Span{
			StartOffset: first.Offset,
			EndOffset:   last.EndOffset,
			StartLine:   first.Pos.Line,
			EndLine:     last.Value.Pos.Line,
		},
	}}
}

// nestedFieldRaw returns the raw scalar text of key's value inside v's own
// block (v.Kind must be KindBlock), if v has an entry with that key -
// deliberately narrow (not a general lookup helper): just enough to pull a
// real event's own "id = ..." out of its containing block. The first match
// wins; a well-formed event never repeats its own id field.
func nestedFieldRaw(v script.Value, key string) (string, bool) {
	if v.Kind != script.KindBlock || v.Block == nil {
		return "", false
	}
	for _, e := range v.Block.Entries {
		if e.Key == key {
			return e.Value.Raw, true
		}
	}
	return "", false
}

// FromLocaleCatalog extracts one Definition per localization entry. Type is
// "localisation/<language>" - the British spelling, matching the real
// on-disk folder name Paradox games actually use (confirmed against a
// real Stellaris install: "localisation/", not "localization/") - a
// generated patch mod's own localisation content (see
// internal/library.GeneratePatch) has to land in the folder the game
// actually reads, not merely a consistently-spelled internal label.
func FromLocaleCatalog(modID, relPath string, c *locale.Catalog) []Definition {
	if c == nil {
		return nil
	}
	defType := Type("localisation/" + c.Language)
	defs := make([]Definition, 0, len(c.Entries))
	for i, e := range c.Entries {
		defs = append(defs, Definition{
			Type:     defType,
			ID:       e.Key,
			ModID:    modID,
			FilePath: relPath,
			Hash:     xhash.Definition([]byte(e.Value)),
			Span: Span{
				StartOffset: e.StartOffset,
				EndOffset:   e.EndOffset,
				StartLine:   e.Line,
				EndLine:     e.Line,
			},
			Order: i,
		})
	}
	return defs
}

// Normalize produces canonical bytes for hashing a script value: it renders
// the value's structure (keys, operators, scalars) while stripping the
// original file's insignificant whitespace, comment text, and formatting, so
// two definitions with identical meaning hash identically even if one mod
// author's file happened to be formatted differently.
func Normalize(v script.Value) []byte {
	var buf bytes.Buffer
	normalizeValue(&buf, v)
	return buf.Bytes()
}

func normalizeValue(buf *bytes.Buffer, v script.Value) {
	switch v.Kind {
	case script.KindBlock:
		buf.WriteByte('{')
		if v.Block != nil {
			for i, e := range v.Block.Entries {
				if i > 0 {
					buf.WriteByte(';')
				}
				normalizeEntry(buf, e)
			}
		}
		buf.WriteByte('}')
	default:
		buf.WriteString(v.Raw)
	}
}

// NormalizeEntry produces canonical bytes for hashing a single script entry
// (key, operator, and value together) - the same normalization Hash itself
// uses internally for a whole top-level object, exported here so a caller
// matching individual entries *within* a definition's block (see
// internal/conflict.AdditiveEntries and docs/merge-patch.md's Tier 1) can
// hash them the same, consistent way rather than duplicating the logic.
func NormalizeEntry(e script.Entry) []byte {
	var buf bytes.Buffer
	normalizeEntry(&buf, e)
	return buf.Bytes()
}

func normalizeEntry(buf *bytes.Buffer, e script.Entry) {
	if e.Key != "" {
		buf.WriteString(e.Key)
		if e.Op == "" {
			buf.WriteByte('=')
		} else {
			buf.WriteString(string(e.Op))
		}
	}
	normalizeValue(buf, e.Value)
}
