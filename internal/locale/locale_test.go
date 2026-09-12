package locale

import "testing"

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
