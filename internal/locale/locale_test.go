package locale

import (
	"strings"
	"testing"
)

func TestParseBasic(t *testing.T) {
	src := []byte("l_english:\n" +
		" NEW_ACHIEVEMENT_2_0_NAME:0 \"Brave New World\"\n" +
		" NEW_ACHIEVEMENT_2_0_DESC:0 \"Colonize a planet\"\n")

	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cat.Language != "english" {
		t.Errorf("Language = %q, want \"english\"", cat.Language)
	}
	if len(cat.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cat.Entries))
	}
	if cat.Entries[0].Key != "NEW_ACHIEVEMENT_2_0_NAME" || cat.Entries[0].Value != "Brave New World" || cat.Entries[0].Version != 0 {
		t.Errorf("entry 0 = %+v", cat.Entries[0])
	}
	if cat.Entries[1].Key != "NEW_ACHIEVEMENT_2_0_DESC" || cat.Entries[1].Value != "Colonize a planet" {
		t.Errorf("entry 1 = %+v", cat.Entries[1])
	}
}

func TestParseStripsUTF8BOM(t *testing.T) {
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte("l_english:\n KEY:0 \"Value\"\n")...)
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cat.Language != "english" {
		t.Errorf("Language = %q, want \"english\"", cat.Language)
	}
	if len(cat.Entries) != 1 || cat.Entries[0].Value != "Value" {
		t.Errorf("Entries = %+v", cat.Entries)
	}
}

func TestParseVersionNumberVariesAndCanBeOmitted(t *testing.T) {
	src := []byte("l_english:\n" +
		" WITH_VERSION:3 \"three\"\n" +
		" NO_VERSION: \"no version given\"\n")
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cat.Entries[0].Version != 3 {
		t.Errorf("Version = %d, want 3", cat.Entries[0].Version)
	}
	if cat.Entries[1].Version != 0 || cat.Entries[1].Value != "no version given" {
		t.Errorf("entry 1 = %+v", cat.Entries[1])
	}
}

func TestParseEscapedQuoteInValue(t *testing.T) {
	src := []byte(`l_english:
 KEY:0 "she said \"hello\""
`)
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := `she said "hello"`
	if cat.Entries[0].Value != want {
		t.Errorf("Value = %q, want %q", cat.Entries[0].Value, want)
	}
}

func TestParseIgnoresBlankLinesAndComments(t *testing.T) {
	src := []byte("l_english:\n" +
		"\n" +
		" # a comment\n" +
		" KEY:0 \"Value\"\n" +
		"\n")
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cat.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(cat.Entries), cat.Entries)
	}
}

func TestParseMissingHeaderErrors(t *testing.T) {
	_, err := Parse([]byte(" KEY:0 \"Value\"\n"))
	if err == nil {
		t.Fatal("expected an error when the language header is missing")
	}
}

func TestParseEmptyInputErrors(t *testing.T) {
	_, err := Parse(nil)
	if err == nil {
		t.Fatal("expected an error for empty input")
	}
}

func TestParseMalformedEntryErrors(t *testing.T) {
	_, err := Parse([]byte("l_english:\n not a valid entry line\n"))
	if err == nil {
		t.Fatal("expected an error for a malformed entry line")
	}
}

func TestParseEntryOffsetsCoverExactRawLine(t *testing.T) {
	src := []byte("l_english:\n" +
		" NEW_ACHIEVEMENT_2_0_NAME:0 \"Brave New World\"\n" +
		" NEW_ACHIEVEMENT_2_0_DESC:0 \"Colonize a planet\"\n")

	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cat.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cat.Entries))
	}
	for i, e := range cat.Entries {
		got := string(src[e.StartOffset:e.EndOffset])
		if !strings.Contains(got, e.Key) || !strings.Contains(got, e.Value) {
			t.Errorf("entry %d: offsets [%d:%d] = %q, want it to contain key %q and value %q", i, e.StartOffset, e.EndOffset, got, e.Key, e.Value)
		}
		// The raw slice must include the original leading indentation
		// (the entry's exact source line, byte-for-byte), not a
		// re-trimmed version.
		if !strings.HasPrefix(got, " ") {
			t.Errorf("entry %d: offsets = %q, want it to preserve the original leading space", i, got)
		}
	}
	want0 := ` NEW_ACHIEVEMENT_2_0_NAME:0 "Brave New World"`
	if got := string(src[cat.Entries[0].StartOffset:cat.Entries[0].EndOffset]); got != want0 {
		t.Errorf("entry 0 raw slice = %q, want %q", got, want0)
	}
}

func TestParseEntryOffsetsStripTrailingCR(t *testing.T) {
	src := []byte("l_english:\r\n KEY:0 \"Value\"\r\n")
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := cat.Entries[0]
	got := string(src[e.StartOffset:e.EndOffset])
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("raw slice = %q, should not include the line terminator", got)
	}
	want := ` KEY:0 "Value"`
	if got != want {
		t.Errorf("raw slice = %q, want %q", got, want)
	}
}

// TestParseEntryOffsetsAreRelativeToOriginalInputWithBOM pins a real bug:
// offsets were originally computed relative to the BOM-stripped view
// Parse works on internally, not the original src the caller passed in -
// silently misaligning any byte range a caller sliced out of the real
// on-disk file it read itself (which still has its BOM). Confirmed
// against real Stellaris locale files, which do carry one.
func TestParseEntryOffsetsAreRelativeToOriginalInputWithBOM(t *testing.T) {
	withoutBOM := "l_english:\n KEY:0 \"Value\"\n"
	src := append([]byte{0xEF, 0xBB, 0xBF}, []byte(withoutBOM)...)

	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := cat.Entries[0]
	want := ` KEY:0 "Value"`
	if got := string(src[e.StartOffset:e.EndOffset]); got != want {
		t.Errorf("src[%d:%d] = %q, want %q (offsets must be relative to the original BOM-prefixed src, not the trimmed view)", e.StartOffset, e.EndOffset, got, want)
	}
}

func TestParseEntryOffsetsOnFinalLineWithNoTrailingNewline(t *testing.T) {
	src := []byte("l_english:\n KEY:0 \"Value\"")
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := cat.Entries[0]
	if e.EndOffset != len(src) {
		t.Errorf("EndOffset = %d, want %d (end of input)", e.EndOffset, len(src))
	}
	want := ` KEY:0 "Value"`
	if got := string(src[e.StartOffset:e.EndOffset]); got != want {
		t.Errorf("raw slice = %q, want %q", got, want)
	}
}
