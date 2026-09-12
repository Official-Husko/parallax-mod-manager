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
// lives in (e.g. "common/buildings", "events", "localization/english").
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

// FromScriptFile extracts one Definition per top-level entry of a parsed
// Clausewitz script file. ID defaults to the entry's own key, which is
// correct for the common case (e.g. common/buildings, where the top-level
// key is the building's own tag). Some categories (e.g. events, where the
// real id is a nested "id = ..." field) need a category-specific override -
// deliberately deferred to the conflict-resolution milestone, not built here.
func FromScriptFile(modID, relPath string, defType Type, f *script.File) []Definition {
	if f == nil {
		return nil
	}
	defs := make([]Definition, 0, len(f.Root.Entries))
	for i, e := range f.Root.Entries {
		if e.Key == "" {
			// A bare list item at the top level of a file has no natural id
			// and isn't a conflictable "object" on its own; skip it.
			continue
		}
		defs = append(defs, Definition{
			Type:     defType,
			ID:       e.Key,
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

// FromLocaleCatalog extracts one Definition per localization entry. Type is
// conventionally "localization/<language>".
func FromLocaleCatalog(modID, relPath string, c *locale.Catalog) []Definition {
	if c == nil {
		return nil
	}
	defType := Type("localization/" + c.Language)
	defs := make([]Definition, 0, len(c.Entries))
	for i, e := range c.Entries {
		defs = append(defs, Definition{
			Type:     defType,
			ID:       e.Key,
			ModID:    modID,
			FilePath: relPath,
			Hash:     xhash.Definition([]byte(e.Value)),
			Span:     Span{StartLine: e.Line, EndLine: e.Line},
			Order:    i,
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
