package definition

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

func mustParseScript(t *testing.T, src string) *script.File {
	t.Helper()
	f, err := script.Parse([]byte(src))
	if err != nil {
		t.Fatalf("script.Parse: %v", err)
	}
	return f
}

func TestFromScriptFileDefaultsIDToTopLevelKey(t *testing.T) {
	f := mustParseScript(t, `
some_building = {
	cost = 100
}
`)
	defs := FromScriptFile("test_mod", "common/buildings/00_buildings.txt", Type("common/buildings"), f)
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	d := defs[0]
	if d.ID != "some_building" {
		t.Errorf("ID = %q, want %q", d.ID, "some_building")
	}
	if d.Type != Type("common/buildings") {
		t.Errorf("Type = %q", d.Type)
	}
	if d.ModID != "test_mod" || d.FilePath != "common/buildings/00_buildings.txt" {
		t.Errorf("ModID/FilePath = %q/%q", d.ModID, d.FilePath)
	}
	if d.Order != 0 {
		t.Errorf("Order = %d, want 0", d.Order)
	}
}

func TestFromScriptFileSkipsBareTopLevelListItems(t *testing.T) {
	// Malformed-ish but should not crash: a bare value with no key at the
	// top level isn't a conflictable object and should be skipped.
	f := mustParseScript(t, `
"a_stray_bare_string"
real_entry = { }
`)
	defs := FromScriptFile("m", "f.txt", "t", f)
	if len(defs) != 1 || defs[0].ID != "real_entry" {
		t.Fatalf("defs = %+v, want exactly one entry for real_entry", defs)
	}
}

func TestFromScriptFilePreservesOrder(t *testing.T) {
	f := mustParseScript(t, `
first = 1
second = 2
third = 3
`)
	defs := FromScriptFile("m", "f.txt", "t", f)
	if len(defs) != 3 {
		t.Fatalf("expected 3 definitions, got %d", len(defs))
	}
	for i, want := range []string{"first", "second", "third"} {
		if defs[i].ID != want || defs[i].Order != i {
			t.Errorf("defs[%d] = %+v, want ID=%q Order=%d", i, defs[i], want, i)
		}
	}
}

func TestFromScriptFileSpanCoversSourceText(t *testing.T) {
	src := "some_building = {\n\tcost = 100\n}\n"
	f := mustParseScript(t, src)
	defs := FromScriptFile("m", "f.txt", "t", f)
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	span := defs[0].Span
	if span.StartOffset != 0 {
		t.Errorf("StartOffset = %d, want 0", span.StartOffset)
	}
	got := src[span.StartOffset:span.EndOffset]
	want := "some_building = {\n\tcost = 100\n}"
	if got != want {
		t.Errorf("span text = %q, want %q", got, want)
	}
}

func TestFromScriptFileNilInput(t *testing.T) {
	if defs := FromScriptFile("m", "f.txt", "t", nil); defs != nil {
		t.Errorf("expected nil for nil file, got %+v", defs)
	}
}

func TestFromLocaleCatalogUsesLanguageInType(t *testing.T) {
	src := []byte("l_english:\n KEY_ONE:0 \"Value One\"\n")
	cat, err := locale.Parse(src)
	if err != nil {
		t.Fatalf("locale.Parse: %v", err)
	}
	defs := FromLocaleCatalog("test_mod", "localisation/english/l_test.yml", cat)
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	d := defs[0]
	// British spelling - matches the real on-disk folder name Paradox
	// games actually use, confirmed against a real Stellaris install.
	if d.Type != Type("localisation/english") {
		t.Errorf("Type = %q, want %q", d.Type, "localisation/english")
	}
	if d.ID != "KEY_ONE" {
		t.Errorf("ID = %q", d.ID)
	}
	if d.Span.EndOffset <= d.Span.StartOffset {
		t.Errorf("Span = %+v, want a real non-empty byte range", d.Span)
	}
	if got := string(src[d.Span.StartOffset:d.Span.EndOffset]); got != ` KEY_ONE:0 "Value One"` {
		t.Errorf("Span slice = %q, want the entry's exact raw source line", got)
	}
}

func TestNormalizeIgnoresWhitespaceAndCommentDifferences(t *testing.T) {
	a := mustParseScript(t, `
x = {
	# a comment
	a = 1
	b = 2
}
`)
	b := mustParseScript(t, `
x={a=1 b=2} # trailing comment, differently spaced
`)
	ha := xhashOf(t, a)
	hb := xhashOf(t, b)
	if ha != hb {
		t.Errorf("expected identical hashes for whitespace/comment-only differences, got %d vs %d", ha, hb)
	}
}

func TestNormalizeDetectsRealContentDifferences(t *testing.T) {
	a := mustParseScript(t, `x = { a = 1 b = 2 }`)
	b := mustParseScript(t, `x = { a = 1 b = 3 }`)
	ha := xhashOf(t, a)
	hb := xhashOf(t, b)
	if ha == hb {
		t.Errorf("expected different hashes for genuinely different content, both = %d", ha)
	}
}

// xhashOf extracts the single top-level entry's definition hash from a
// parsed file, for comparing two fixtures' normalized content.
func xhashOf(t *testing.T, f *script.File) uint64 {
	t.Helper()
	defs := FromScriptFile("m", "f.txt", "t", f)
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 top-level entry, got %d", len(defs))
	}
	return defs[0].Hash
}
