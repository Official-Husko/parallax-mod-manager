package locale

import (
	"bytes"
	"testing"
)

func TestRenderFileStartsWithTheUTF8BOM(t *testing.T) {
	got := RenderFile("english", map[string]string{"A_KEY": "value"})
	if !bytes.HasPrefix(got, utf8BOM) {
		t.Errorf("RenderFile output does not start with the UTF-8 BOM: %q", got[:min(10, len(got))])
	}
}

func TestRenderFileWritesTheLanguageHeader(t *testing.T) {
	got := RenderFile("german", map[string]string{"A_KEY": "value"})
	if !bytes.Contains(got, []byte("l_german:\n")) {
		t.Errorf("RenderFile output missing the l_german: header: %q", got)
	}
}

func TestRenderFileSortsKeys(t *testing.T) {
	got := string(RenderFile("english", map[string]string{"ZEBRA": "z", "APPLE": "a", "MANGO": "m"}))
	apple := indexOf(got, "APPLE")
	mango := indexOf(got, "MANGO")
	zebra := indexOf(got, "ZEBRA")
	if !(apple < mango && mango < zebra) {
		t.Errorf("keys not sorted: APPLE@%d MANGO@%d ZEBRA@%d", apple, mango, zebra)
	}
}

func TestRenderFileEscapesLiteralQuotes(t *testing.T) {
	got := string(RenderFile("english", map[string]string{"K": `she said "hi"`}))
	if !bytes.Contains([]byte(got), []byte(`K:0 "she said \"hi\""`)) {
		t.Errorf("quotes not escaped correctly: %q", got)
	}
}

func TestRenderFileRoundTripsThroughParse(t *testing.T) {
	original := map[string]string{
		"GREETING":     "Hello, \"friend\"!",
		"FAREWELL":     "Goodbye",
		"WITH_NEWLINE": "line one\nline two",
	}
	rendered := RenderFile("english", original)

	cat, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Parse(RenderFile(...)) error = %v", err)
	}
	if cat.Language != "english" {
		t.Errorf("Language = %q, want \"english\"", cat.Language)
	}
	if len(cat.Skipped) != 0 {
		t.Errorf("Skipped = %v, want none", cat.Skipped)
	}
	got := map[string]string{}
	for _, e := range cat.Entries {
		got[e.Key] = e.Value
	}
	if got["GREETING"] != `Hello, "friend"!` {
		t.Errorf("GREETING round-tripped as %q", got["GREETING"])
	}
	if got["FAREWELL"] != "Goodbye" {
		t.Errorf("FAREWELL round-tripped as %q", got["FAREWELL"])
	}
	if got["WITH_NEWLINE"] != "line one line two" {
		t.Errorf("WITH_NEWLINE round-tripped as %q, want the embedded newline flattened to a space", got["WITH_NEWLINE"])
	}
}

func TestRenderFileOnEmptyEntriesIsStillAValidParsableFile(t *testing.T) {
	rendered := RenderFile("english", map[string]string{})
	cat, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Parse(RenderFile(empty)) error = %v", err)
	}
	if len(cat.Entries) != 0 {
		t.Errorf("Entries = %v, want none", cat.Entries)
	}
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
