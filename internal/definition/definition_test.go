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

func TestFromScriptFileEventsUsesNestedIDField(t *testing.T) {
	// Every real event's top-level key is one of a handful of generic
	// category keywords ("country_event", "ship_event", ...) shared by
	// thousands of unrelated events - the real, unique id is nested inside
	// as "id = ...". Two different events sharing the same category
	// keyword must not collapse to the same ID.
	f := mustParseScript(t, `
country_event = {
	id = my_events.1
	title = my_events.1.title
}
country_event = {
	id = my_events.2
	title = my_events.2.title
}
`)
	defs := FromScriptFile("test_mod", "events/x.txt", eventsType, f)
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d: %+v", len(defs), defs)
	}
	if defs[0].ID != "my_events.1" || defs[1].ID != "my_events.2" {
		t.Errorf("IDs = %q, %q, want my_events.1, my_events.2", defs[0].ID, defs[1].ID)
	}
	for _, d := range defs {
		if d.Type != eventsType {
			t.Errorf("Type = %q, want %q", d.Type, eventsType)
		}
	}
}

func TestFromScriptFileSectionTemplatesUsesNestedKeyField(t *testing.T) {
	// Every real section template's top-level key is the single literal
	// keyword "ship_section_template", shared by hundreds of unrelated
	// vanilla templates alone - the real, unique name is nested as
	// "key = ...". Two different templates sharing that same wrapper
	// keyword must not collapse to the same ID.
	f := mustParseScript(t, `
ship_section_template = {
	key = "TITAN_BOW"
	ship_size = titan
}
ship_section_template = {
	key = "TITAN_CORE"
	ship_size = titan
}
`)
	const sectionTemplatesType = Type("common/section_templates")
	defs := FromScriptFile("test_mod", "common/section_templates/x.txt", sectionTemplatesType, f)
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d: %+v", len(defs), defs)
	}
	if defs[0].ID != "TITAN_BOW" || defs[1].ID != "TITAN_CORE" {
		t.Errorf("IDs = %q, %q, want TITAN_BOW, TITAN_CORE", defs[0].ID, defs[1].ID)
	}
}

func TestFromScriptFileMessageTypesUsesNestedKeyField(t *testing.T) {
	f := mustParseScript(t, `
message_type = {
	key = "FIRST_CONTACT"
}
message_type = {
	key = "SECOND_CONTACT"
}
`)
	const messageTypesType = Type("common/message_types")
	defs := FromScriptFile("test_mod", "common/message_types/x.txt", messageTypesType, f)
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d: %+v", len(defs), defs)
	}
	if defs[0].ID != "FIRST_CONTACT" || defs[1].ID != "SECOND_CONTACT" {
		t.Errorf("IDs = %q, %q, want FIRST_CONTACT, SECOND_CONTACT", defs[0].ID, defs[1].ID)
	}
}

func TestFromScriptFileWholeFileTypeUsesFilenameAsID(t *testing.T) {
	// Every top-level entry in a real inline_scripts file is just an
	// ordinary field (icon = ..., modifier = {...}) - no wrapper key at
	// all. The whole file is one unit; its own filename (without
	// extension) is its real identity, per the game's own documentation.
	f := mustParseScript(t, `
icon = "some/path.dds"
modifier = {
	a = 1
}
`)
	defs := FromScriptFile("test_mod", "common/inline_scripts/traits/radiotrophic_effects.txt", Type("common/inline_scripts/traits"), f)
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 definition for the whole file, got %d: %+v", len(defs), defs)
	}
	if defs[0].ID != "radiotrophic_effects" {
		t.Errorf("ID = %q, want radiotrophic_effects (the filename, not a field name)", defs[0].ID)
	}
}

func TestFromScriptFileWholeFileTypeAppliesToSubfoldersToo(t *testing.T) {
	f := mustParseScript(t, `resources = { category = edicts }`)
	defs := FromScriptFile("test_mod", "common/inline_scripts/edicts/upkeep_low.txt", Type("common/inline_scripts/edicts"), f)
	if len(defs) != 1 || defs[0].ID != "upkeep_low" {
		t.Fatalf("defs = %+v, want exactly one entry with ID upkeep_low", defs)
	}
}

func TestFromScriptFileWholeFileTypeDirectlyUnderTheFolderToo(t *testing.T) {
	// Not every inline script lives in a subfolder - some sit directly
	// under common/inline_scripts itself (Type has no further "/subfolder"
	// suffix at all), which must still be treated as whole-file.
	f := mustParseScript(t, `weight = 10`)
	defs := FromScriptFile("test_mod", "common/inline_scripts/councilor_leader_weights.txt", Type("common/inline_scripts"), f)
	if len(defs) != 1 || defs[0].ID != "councilor_leader_weights" {
		t.Fatalf("defs = %+v, want exactly one entry with ID councilor_leader_weights", defs)
	}
}

func TestFromScriptFileWholeFileTypeEmptyFileYieldsNoDefinitions(t *testing.T) {
	f := mustParseScript(t, `# just a comment, nothing else`)
	defs := FromScriptFile("test_mod", "common/inline_scripts/traits/empty.txt", Type("common/inline_scripts/traits"), f)
	if len(defs) != 0 {
		t.Errorf("defs = %+v, want none for an empty file", defs)
	}
}

func TestFromScriptFileWholeFileTypeHashCoversTheWholeFile(t *testing.T) {
	a := mustParseScript(t, `icon = "a" modifier = { x = 1 }`)
	b := mustParseScript(t, `icon = "a" modifier = { x = 2 }`)       // differs only in a later field
	same := mustParseScript(t, `icon = "a"    modifier = { x = 1 }`) // whitespace-only difference

	defsA := FromScriptFile("m", "common/inline_scripts/traits/x.txt", Type("common/inline_scripts/traits"), a)
	defsB := FromScriptFile("m", "common/inline_scripts/traits/x.txt", Type("common/inline_scripts/traits"), b)
	defsSame := FromScriptFile("m", "common/inline_scripts/traits/x.txt", Type("common/inline_scripts/traits"), same)

	if defsA[0].Hash == defsB[0].Hash {
		t.Error("expected different hashes - the two files genuinely differ")
	}
	if defsA[0].Hash != defsSame[0].Hash {
		t.Error("expected identical hashes - only whitespace differs")
	}
}

func TestIsWholeFileType(t *testing.T) {
	tests := []struct {
		typ  Type
		want bool
	}{
		{"common/inline_scripts", true},
		{"common/inline_scripts/traits", true},
		{"common/inline_scripts/buildings", true},
		{"common/buildings", false},
		{"common/inline_scripts_other", false}, // must match a whole segment, not a prefix of the folder name itself
	}
	for _, tt := range tests {
		if got := IsWholeFileType(tt.typ); got != tt.want {
			t.Errorf("IsWholeFileType(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestFromScriptFileEventsSkipsNamespaceDeclaration(t *testing.T) {
	f := mustParseScript(t, `
namespace = my_events
country_event = {
	id = my_events.1
}
`)
	defs := FromScriptFile("test_mod", "events/x.txt", eventsType, f)
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition (namespace skipped), got %d: %+v", len(defs), defs)
	}
	if defs[0].ID != "my_events.1" {
		t.Errorf("ID = %q, want my_events.1", defs[0].ID)
	}
}

func TestFromScriptFileEventsWithoutNestedIDFallsBackToTopLevelKey(t *testing.T) {
	// inline_script (and any other malformed-ish events-folder entry with
	// no "id" field of its own) keeps the old behavior rather than
	// crashing or silently dropping the entry.
	f := mustParseScript(t, `
inline_script = {
	script = "some/path"
}
`)
	defs := FromScriptFile("test_mod", "events/x.txt", eventsType, f)
	if len(defs) != 1 || defs[0].ID != "inline_script" {
		t.Fatalf("defs = %+v, want exactly one entry with ID inline_script", defs)
	}
}

func TestFromScriptFileNonEventsTypeUnaffectedByEventHandling(t *testing.T) {
	// A non-events Type must never look inside its own block for a nested
	// "id" field, even if one happens to be present - only defType ==
	// eventsType triggers that behavior.
	f := mustParseScript(t, `
some_building = {
	id = should_not_be_used
	cost = 100
}
`)
	defs := FromScriptFile("test_mod", "common/buildings/x.txt", Type("common/buildings"), f)
	if len(defs) != 1 || defs[0].ID != "some_building" {
		t.Fatalf("defs = %+v, want exactly one entry with ID some_building (unaffected by its own nested id field)", defs)
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
