package loverslabtracking

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "loverslab_installs")}
	installs := map[string]Entry{
		"loverslab_31347": {FileURL: "https://www.loverslab.com/files/file/31347-stable-portraits/", FileID: 31347, Title: "Stable Portraits", InstalledDateModified: "2 days ago", InstalledAt: 1758000000},
		"loverslab_44226": {FileURL: "https://www.loverslab.com/files/file/44226-deluxe-species-pack-reforged/", FileID: 44226, Title: "Deluxe Species pack reforged, with \"quotes\"", InstalledDateModified: "September 13", InstalledAt: 1758000001},
	}
	if err := s.Save("g1", installs); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load("g1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, installs) {
		t.Errorf("Load = %+v, want %+v", got, installs)
	}
}

func TestTheFileIsCommentedJSONCInIDOrder(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := s.Save("g1", map[string]Entry{
		"loverslab_2": {FileURL: "https://www.loverslab.com/files/file/2-b/", Title: "B"},
		"loverslab_1": {FileURL: "https://www.loverslab.com/files/file/1-a/", Title: "A"},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "g1.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "// Parallax Mod Manager") || !strings.Contains(text, "update check compares") {
		t.Errorf("the file does not explain itself:\n%s", text)
	}
	if strings.Index(text, `"loverslab_1"`) > strings.Index(text, `"loverslab_2"`) {
		t.Errorf("entries are not in ID order:\n%s", text)
	}
	if strings.Contains(text, "—") {
		t.Error("em dash in the generated file")
	}
}

func TestNoFileMeansNoInstalls(t *testing.T) {
	got, err := Store{Dir: t.TempDir()}.Load("g1")
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("Load = %v, %v; want an empty map", got, err)
	}
	if got, err := (Store{}).Load("g1"); err != nil || len(got) != 0 {
		t.Errorf("no folder: %v, %v", got, err)
	}
}

func TestAFileThatCannotBeReadIsAnErrorAndIsNotOverwrittenByLoading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "g1.jsonc")
	broken := `{"installs": {"a": {"fileUrl": "precious" `
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
	body := `// my tracked installs
{
  "installs": {
    // the good one
    "loverslab_1": {"fileUrl": "https://www.loverslab.com/files/file/1-a/", "fileId": 1, "title": "One", "installedDateModified": "today", "installedAt": 1,},
    "loverslab_2": {"fileUrl": "   ", "fileId": 2, "title": "Blank URL - not real", "installedDateModified": "", "installedAt": 0},  // no real entry
  },
}`
	if err := os.WriteFile(filepath.Join(dir, "g1.jsonc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Store{Dir: dir}.Load("g1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Entry{"loverslab_1": {FileURL: "https://www.loverslab.com/files/file/1-a/", FileID: 1, Title: "One", InstalledDateModified: "today", InstalledAt: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestGamesKeepSeparateFiles(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	_ = s.Save("g1", map[string]Entry{"a": {FileURL: "https://www.loverslab.com/files/file/1-a/", Title: "one"}})
	_ = s.Save("g2", map[string]Entry{"a": {FileURL: "https://www.loverslab.com/files/file/2-a/", Title: "two"}})
	g1, _ := s.Load("g1")
	g2, _ := s.Load("g2")
	if g1["a"].Title != "one" || g2["a"].Title != "two" {
		t.Errorf("g1 = %v, g2 = %v", g1, g2)
	}
}

func TestWith(t *testing.T) {
	base := map[string]Entry{"a": {FileURL: "https://www.loverslab.com/files/file/1-a/", Title: "keep"}}
	newEntry := Entry{FileURL: "https://www.loverslab.com/files/file/2-b/", FileID: 2, Title: "Bee", InstalledDateModified: "today", InstalledAt: 100}
	got, err := With(base, "b", newEntry)
	if err != nil {
		t.Fatal(err)
	}
	if got["b"] != newEntry || got["a"].Title != "keep" {
		t.Errorf("With = %+v", got)
	}
	if _, touched := base["b"]; touched {
		t.Error("With modified its input")
	}
	// Overwriting.
	updated := Entry{FileURL: "https://www.loverslab.com/files/file/2-b/", FileID: 2, Title: "Bee", InstalledDateModified: "yesterday", InstalledAt: 200}
	got, _ = With(got, "b", updated)
	if got["b"].InstalledDateModified != "yesterday" {
		t.Errorf("overwrite: %+v", got)
	}
	// A zero-value entry (empty FileURL) removes it.
	got, _ = With(got, "b", Entry{})
	if _, ok := got["b"]; ok || len(got) != 1 {
		t.Errorf("empty FileURL should remove the entry: %+v", got)
	}
	if _, err := With(base, "  ", newEntry); err == nil {
		t.Error("an entry needs a mod")
	}
}
