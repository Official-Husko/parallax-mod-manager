package translatecache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	rec := Record{
		Entries: map[string]map[string]Entry{
			"DE": {
				"GREETING": {SourceHash: "abc123", TranslatedText: "Hallo", Service: "deepl", TranslatedAt: 1700000000},
			},
		},
	}
	if err := Save(dir, "stellaris", "my_mod", rec); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, ok := Load(dir, "stellaris", "my_mod")
	if !ok {
		t.Fatal("Load() reported not found after a successful Save")
	}
	entry, ok := got.Entries["DE"]["GREETING"]
	if !ok {
		t.Fatal("Load() is missing the saved DE/GREETING entry")
	}
	if entry.TranslatedText != "Hallo" || entry.Service != "deepl" {
		t.Errorf("entry = %+v, want TranslatedText=Hallo Service=deepl", entry)
	}
}

func TestLoadOnAMissingFileReportsNotFoundNotAnError(t *testing.T) {
	_, ok := Load(t.TempDir(), "stellaris", "no_such_mod")
	if ok {
		t.Error("Load() on a missing file reported found")
	}
}

func TestLoadOnACorruptFileReportsNotFound(t *testing.T) {
	dir := t.TempDir()
	p := path(dir, "stellaris", "my_mod")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(dir, "stellaris", "my_mod"); ok {
		t.Error("Load() on a corrupt file reported found")
	}
}

func TestLoadRefusesAFileFromANewerFormatVersion(t *testing.T) {
	dir := t.TempDir()
	rec := Record{FormatVersion: FormatVersion + 1, Entries: map[string]map[string]Entry{}}
	if err := Save(dir, "stellaris", "my_mod", rec); err != nil {
		t.Fatal(err)
	}
	// Save always stamps the current FormatVersion, so overwrite the file
	// by hand afterward to simulate a genuinely newer file.
	p := path(dir, "stellaris", "my_mod")
	if err := os.WriteFile(p, []byte(`{"formatVersion": 999, "entries": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(dir, "stellaris", "my_mod"); ok {
		t.Error("Load() accepted a file claiming a newer format version than this build understands")
	}
}

func TestSaveFailsCleanlyWithNoConfigDir(t *testing.T) {
	if err := Save("", "stellaris", "my_mod", Record{}); err == nil {
		t.Error("Save() with an empty configDir should fail rather than write somewhere unexpected")
	}
}

func TestPathIsScopedByGameAndSanitizedModID(t *testing.T) {
	p1 := path("/cfg", "stellaris", "mod/with/slashes")
	p2 := path("/cfg", "eu4", "mod/with/slashes")
	if p1 == p2 {
		t.Error("path() did not differ between two different games for the same mod ID")
	}
	if filepath.Base(p1) != "mod_with_slashes.jsonc" {
		t.Errorf("path() base = %q, want slashes sanitized to underscores", filepath.Base(p1))
	}
}

func TestSaveTwiceReplacesRatherThanMerges(t *testing.T) {
	dir := t.TempDir()
	first := Record{Entries: map[string]map[string]Entry{"DE": {"A": {TranslatedText: "eins"}}}}
	if err := Save(dir, "stellaris", "my_mod", first); err != nil {
		t.Fatal(err)
	}
	second := Record{Entries: map[string]map[string]Entry{"DE": {"B": {TranslatedText: "zwei"}}}}
	if err := Save(dir, "stellaris", "my_mod", second); err != nil {
		t.Fatal(err)
	}
	got, ok := Load(dir, "stellaris", "my_mod")
	if !ok {
		t.Fatal("Load() reported not found")
	}
	if _, stillThere := got.Entries["DE"]["A"]; stillThere {
		t.Error("Save() merged with the previous file instead of replacing it - caller (translate_plan.go) is documented to always pass the full merged Record itself")
	}
	if _, present := got.Entries["DE"]["B"]; !present {
		t.Error("Save()'s second write did not take effect")
	}
}
