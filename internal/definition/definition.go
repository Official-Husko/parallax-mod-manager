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

// eventsType is the Type FromScriptFile treats specially: every top-level
// entry in a real events file is one of a handful of generic category
// keywords ("country_event", "ship_event", "fleet_event", ...) shared by
// thousands of unrelated events (confirmed against a real Stellaris
// install: "country_event" alone is the literal top-level key of 5,000+
// distinct vanilla events) - never itself a usable id. This convention
// (a nested "id = <namespace>.<number>" field carries the real, globally
// unique identifier; the top-level key only says which event category it
// is) is a long-standing, near-universal Clausewitz one, confirmed here
// against Stellaris specifically and assumed, not independently reverified,
// to hold for every other Paradox game's own identically-named "events"
// folder.
const eventsType = Type("events")

// eventIDField is the nested field a real event's own Definition carries
// its true id in - see eventsType.
const eventIDField = "id"

// eventNamespaceKey is the bare top-level directive ("namespace = foo")
// every real events file starts with, establishing the file's own id
// prefix. It carries no content of its own to conflict over, and its
// literal key repeats across virtually every mod's own events files (160+
// times in the vanilla install's own events/ folder alone) - treating it
// as a definition would flag a bogus conflict between any two mods that
// each declare one, the same problem eventsType/eventIDField above solves
// for actual events. Skipped entirely rather than assigned any id.
const eventNamespaceKey = "namespace"

// FromScriptFile extracts one Definition per top-level entry of a parsed
// Clausewitz script file. ID defaults to the entry's own key, which is
// correct for the common case (e.g. common/buildings, where the top-level
// key is the building's own tag). defType == eventsType is a confirmed
// exception - see eventsType/eventIDField/eventNamespaceKey.
func FromScriptFile(modID, relPath string, defType Type, f *script.File) []Definition {
	if f == nil {
		return nil
	}
	isEvents := defType == eventsType
	defs := make([]Definition, 0, len(f.Root.Entries))
	for i, e := range f.Root.Entries {
		if e.Key == "" {
			// A bare list item at the top level of a file has no natural id
			// and isn't a conflictable "object" on its own; skip it.
			continue
		}
		if isEvents && e.Key == eventNamespaceKey {
			continue
		}
		id := e.Key
		if isEvents {
			if nested, ok := nestedFieldRaw(e.Value, eventIDField); ok {
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
