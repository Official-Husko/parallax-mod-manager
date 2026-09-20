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

func TestParseValueMayBeFollowedByAComment(t *testing.T) {
	src := "l_english:\n" +
		" A:0 \"Tion Hegemony\" # Player\n" +
		" B:0 \"No space\"#tight\n" +
		" C:1 \"tabbed\"\t# note\n" +
		" D:0 \"plain, no comment\"\n" +
		" E:0 \"Number #1 is part of the text\" # but this is a comment\n" +
		" F:0 \"has an \\\"escaped\\\" quote\" # and a comment\n" +
		" G:0 \"quote \"inside\" then more\" # comment\n" +
		" H:0 \"comment holds quotes\" # said \"hello\" to \"it\"\n"
	cat, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[string]string{
		"A": "Tion Hegemony",
		"B": "No space",
		"C": "tabbed",
		"D": "plain, no comment",
		"E": "Number #1 is part of the text",
		"F": `has an "escaped" quote`,
		"G": `quote "inside" then more`,
		"H": "comment holds quotes",
	}
	if len(cat.Entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(cat.Entries), len(want), cat.Entries)
	}
	for _, e := range cat.Entries {
		if e.Value != want[e.Key] {
			t.Errorf("%s = %q, want %q", e.Key, e.Value, want[e.Key])
		}
	}
}

func TestParseCommentDoesNotChangeTheValueOrTheEntryRange(t *testing.T) {
	plain := " KEY:0 \"Same text\"\n"
	commented := " KEY:0 \"Same text\" # a note\n"
	a, err := Parse([]byte("l_english:\n" + plain))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse([]byte("l_english:\n" + commented))
	if err != nil {
		t.Fatal(err)
	}
	if a.Entries[0].Value != b.Entries[0].Value {
		t.Errorf("a trailing comment changed the value: %q vs %q - it would make two mods with the same text look like a conflict", a.Entries[0].Value, b.Entries[0].Value)
	}
	src := []byte("l_english:\n" + commented)
	e := b.Entries[0]
	if got := string(src[e.StartOffset:e.EndOffset]); got != strings.TrimSuffix(commented, "\n") {
		t.Errorf("the entry's byte range = %q, want the whole raw line including its comment", got)
	}
}

func TestParseStillRejectsAValueWithJunkAfterTheQuote(t *testing.T) {
	for _, line := range []string{
		` KEY:0 "text" junk`,
		` KEY:0 "text" and more "quoted"  x`,
		` KEY:0 "never closed`,
		` KEY:0 unquoted`,
	} {
		if _, err := Parse([]byte("l_english:\n" + line + "\n")); err == nil {
			t.Errorf("%q should not parse", line)
		}
	}
}

func TestParseKeepsAcceptingWhatItAlwaysAccepted(t *testing.T) {
	// A value ending in a backslash right before the closing quote: read
	// literally, as it always was.
	cat, err := Parse([]byte("l_english:\n KEY:0 \"ends with a slash\\\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cat.Entries[0].Value != `ends with a slash\` {
		t.Errorf("Value = %q", cat.Entries[0].Value)
	}
}

func TestParseStripsRepeatedBOMs(t *testing.T) {
	src := append([]byte{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF}, []byte("l_french:\n KEY:0 \"valeur\"\n")...)
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cat.Language != "french" || len(cat.Entries) != 1 {
		t.Fatalf("catalog = %+v", cat)
	}
	e := cat.Entries[0]
	if got := string(src[e.StartOffset:e.EndOffset]); got != ` KEY:0 "valeur"` {
		t.Errorf("with two BOMs the entry's range must still index the original bytes, got %q", got)
	}
}

func TestParseSkipsABadLineAndKeepsTheRestOfTheFile(t *testing.T) {
	src := []byte("l_english:\n" +
		" GOOD_ONE:0 \"first\"\n" +
		" RUNS_OVER:0 \"a value that continues\n" +
		"onto a second line\"\n" +
		" STRAY:0 \"closed\".\n" +
		" GOOD_TWO:0 \"second\"\n")
	cat, err := Parse(src)
	if err != nil {
		t.Fatalf("a file with a few bad lines must still parse: %v", err)
	}
	var keys []string
	for _, e := range cat.Entries {
		keys = append(keys, e.Key)
	}
	if strings.Join(keys, ",") != "GOOD_ONE,GOOD_TWO" {
		t.Errorf("entries = %v, want the two good ones", keys)
	}
	if len(cat.Skipped) != 3 {
		t.Fatalf("skipped = %+v, want the three unreadable lines (the wrapped value's two lines and the stray one)", cat.Skipped)
	}
	if cat.Skipped[0].Line != 3 || cat.Skipped[1].Line != 4 || cat.Skipped[2].Line != 5 {
		t.Errorf("skipped lines = %+v, want 3, 4 and 5", cat.Skipped)
	}
	// The entries after a bad line still point at their own bytes.
	for _, e := range cat.Entries {
		if line := string(src[e.StartOffset:e.EndOffset]); !strings.Contains(line, e.Key+":0") {
			t.Errorf("entry %s has the wrong byte range %q", e.Key, line)
		}
	}
}

func TestParseWithNothingReadableIsStillAnError(t *testing.T) {
	if _, err := Parse([]byte("l_english:\n just some prose\n and more prose\n")); err == nil {
		t.Error("a file where no line is an entry isn't a localisation file with a typo - it must still fail")
	}
}

func TestParseHeaderProblemsStayFatal(t *testing.T) {
	if _, err := Parse([]byte(" KEY:0 \"v\"\n")); err == nil {
		t.Error("a missing header must still be an error")
	}
}
