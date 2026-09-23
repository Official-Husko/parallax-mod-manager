package loverslabmeta

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	entries := map[int]Entry{
		8719:  {AuthorAvatarURL: "https://static.loverslab.com/a.jpg", Views: 1000, Updated: "2026-09-13T20:01:52+0200", CachedAt: 1758000000},
		31347: {AuthorAvatarURL: "", Views: 33402, Updated: "2026-09-13T20:01:52+0200", CachedAt: 1758000001},
	}
	if err := s.Save(entries); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, entries) {
		t.Errorf("Load = %+v, want %+v", got, entries)
	}
}

func TestNoFileMeansNoEntries(t *testing.T) {
	got, err := Store{Dir: t.TempDir()}.Load()
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("Load = %v, %v; want an empty map", got, err)
	}
	if got, err := (Store{}).Load(); err != nil || len(got) != 0 {
		t.Errorf("no folder: %v, %v", got, err)
	}
}

func TestTheFileIsCommentedJSONCInIDOrder(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := s.Save(map[int]Entry{
		2: {Views: 2},
		1: {Views: 1},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "cache.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "// Parallax Mod Manager") {
		t.Errorf("the file does not explain itself:\n%s", text)
	}
	if strings.Index(text, `"1"`) > strings.Index(text, `"2"`) {
		t.Errorf("entries are not in ID order:\n%s", text)
	}
	if strings.Contains(text, "—") {
		t.Error("em dash in the generated file")
	}
}

func TestAFileThatCannotBeReadIsAnErrorAndIsNotOverwrittenByLoading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonc")
	broken := `{"files": {"1": {"views": `
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: dir}).Load(); err == nil || !strings.Contains(err.Error(), "fix or remove it by hand") {
		t.Errorf("Load error = %v, want one saying to fix the file", err)
	}
	if data, _ := os.ReadFile(path); string(data) != broken {
		t.Errorf("Load changed the file: %q", data)
	}
}

func TestHandWrittenJSONCIsRead(t *testing.T) {
	dir := t.TempDir()
	body := `// my cache
{
  "files": {
    // the good one
    "8719": {"authorAvatarUrl": "https://static.loverslab.com/a.jpg", "views": 1000, "updated": "2026-09-13T20:01:52+0200", "cachedAt": 1,},
  },
}`
	if err := os.WriteFile(filepath.Join(dir, "cache.jsonc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Store{Dir: dir}.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]Entry{8719: {AuthorAvatarURL: "https://static.loverslab.com/a.jpg", Views: 1000, Updated: "2026-09-13T20:01:52+0200", CachedAt: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestWithAddsAndOverwritesWithoutModifyingItsInput(t *testing.T) {
	base := map[int]Entry{1: {Views: 100}}
	got := With(base, 2, Entry{Views: 200})
	if got[2].Views != 200 || got[1].Views != 100 {
		t.Errorf("With = %+v", got)
	}
	if _, touched := base[2]; touched {
		t.Error("With modified its input")
	}
	got = With(got, 2, Entry{Views: 250})
	if got[2].Views != 250 {
		t.Errorf("overwrite: %+v", got)
	}
}

func TestWithEvictsTheOldestTenthOnceOverTheCap(t *testing.T) {
	entries := make(map[int]Entry, maxEntries)
	for i := 0; i < maxEntries; i++ {
		entries[i] = Entry{CachedAt: int64(i)} // id 0 is oldest, maxEntries-1 is newest
	}
	got := With(entries, -1, Entry{CachedAt: int64(maxEntries)})
	if len(got) > maxEntries {
		t.Errorf("len(got) = %d, want at most %d after eviction", len(got), maxEntries)
	}
	if _, ok := got[0]; ok {
		t.Error("the oldest entry should have been evicted")
	}
	if _, ok := got[maxEntries-1]; !ok {
		t.Error("the newest existing entry should never be evicted")
	}
	if _, ok := got[-1]; !ok {
		t.Error("the new entry that triggered eviction should still be present")
	}
}
