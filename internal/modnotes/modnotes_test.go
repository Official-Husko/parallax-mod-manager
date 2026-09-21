package modnotes

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveThenLoadRoundTripsIncludingAwkwardText(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "mod_notes")}
	notes := map[string]Entry{
		"ugc_1506081116": {Name: "Flat Nameplates", Text: "line one\nline two with \"quotes\", a \\ backslash and // slashes\n\ttabbed"},
		"Lustful Void":   {Name: "Lustful Void", Text: "unicode: ünïcödé ✓ 日本語"},
	}
	if err := s.Save("g1", notes); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load("g1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, notes) {
		t.Errorf("Load = %+v, want %+v", got, notes)
	}
}

func TestTheFileIsCommentedJSONCInIDOrder(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := s.Save("g1", map[string]Entry{"b": {Name: "B", Text: "second"}, "a": {Name: "A", Text: "first"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "g1.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "// Parallax Mod Manager") || !strings.Contains(text, `"name" only helps you`) {
		t.Errorf("the file does not explain itself:\n%s", text)
	}
	if strings.Index(text, `"a"`) > strings.Index(text, `"b"`) {
		t.Errorf("entries are not in ID order:\n%s", text)
	}
	if strings.Contains(text, "\u2014") {
		t.Error("em dash in the generated file")
	}
}

func TestNoFileMeansNoNotes(t *testing.T) {
	got, err := Store{Dir: t.TempDir()}.Load("g1")
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("Load = %v, %v; want an empty map", got, err)
	}
	if got, err := (Store{}).Load("g1"); err != nil || len(got) != 0 {
		t.Errorf("no folder: %v, %v", got, err)
	}
}

// The notes are the person's writing: a broken file is an error, and is left alone.
func TestAFileThatCannotBeReadIsAnErrorAndIsNotOverwrittenByLoading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "g1.jsonc")
	broken := `{"notes": {"a": {"name": "A", "text": "precious" `
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: dir}).Load("g1"); err == nil || !strings.Contains(err.Error(), "fix or remove it by hand") {
		t.Errorf("Load error = %v, want one saying to fix the file", err)
	}
	if data, _ := os.ReadFile(path); string(data) != broken {
		t.Errorf("Load changed the file: %q", data)
	}
}

func TestHandWrittenJSONCIsRead(t *testing.T) {
	dir := t.TempDir()
	body := `// my notes
{
  "notes": {
    // the big one
    "ugc_1": {"name": "One", "text": "hand written",},
    "ugc_2": {"name": "Two", "text": "   "},  // blank: no note
  },
}`
	if err := os.WriteFile(filepath.Join(dir, "g1.jsonc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Store{Dir: dir}.Load("g1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Entry{"ugc_1": {Name: "One", Text: "hand written"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestGamesKeepSeparateFiles(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	_ = s.Save("g1", map[string]Entry{"a": {Text: "one"}})
	_ = s.Save("g2", map[string]Entry{"a": {Text: "two"}})
	g1, _ := s.Load("g1")
	g2, _ := s.Load("g2")
	if g1["a"].Text != "one" || g2["a"].Text != "two" {
		t.Errorf("g1 = %v, g2 = %v", g1, g2)
	}
}

func TestWith(t *testing.T) {
	base := map[string]Entry{"a": {Name: "A", Text: "keep"}}
	got, err := With(base, "b", "  Bee ", "  \r\nhello\r\nworld \n ")
	if err != nil {
		t.Fatal(err)
	}
	if got["b"] != (Entry{Name: "Bee", Text: "hello\nworld"}) || got["a"].Text != "keep" {
		t.Errorf("With = %+v", got)
	}
	if _, touched := base["b"]; touched {
		t.Error("With modified its input")
	}
	// Overwriting.
	got, _ = With(got, "b", "Bee", "changed")
	if got["b"].Text != "changed" {
		t.Errorf("overwrite: %+v", got)
	}
	// Empty removes.
	got, _ = With(got, "b", "Bee", "  \n ")
	if _, ok := got["b"]; ok || len(got) != 1 {
		t.Errorf("empty text should remove the note: %+v", got)
	}
	if _, err := With(base, "  ", "x", "y"); err == nil {
		t.Error("a note needs a mod")
	}
}

func TestLengthLimitCountsCharactersNotBytes(t *testing.T) {
	ok := strings.Repeat("日", MaxLength)
	if _, err := Clean(ok); err != nil {
		t.Errorf("exactly the limit in multi-byte characters was refused: %v", err)
	}
	if _, err := Clean(ok + "x"); !errors.Is(err, ErrTooLong) {
		t.Errorf("one over the limit: %v, want ErrTooLong", err)
	}
	if _, err := With(nil, "a", "A", strings.Repeat("x", MaxLength+1)); !errors.Is(err, ErrTooLong) {
		t.Errorf("With over the limit: %v", err)
	}
}
