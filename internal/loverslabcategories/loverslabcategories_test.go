package loverslabcategories

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
)

func sampleCategories() []loverslab.Category {
	return []loverslab.Category{
		{ID: 194, Name: "All", URL: "https://www.loverslab.com/files/category/194-paradox-games/", Files: 900, Depth: 0},
		{ID: 1, Name: "Stellaris", URL: "https://www.loverslab.com/files/category/1-stellaris/", Files: 620, Depth: 1},
	}
}

func TestSaveThenLoadRoundTripsAndIsFresh(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	want := sampleCategories()
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, fresh, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
	if !fresh {
		t.Error("a cache just saved should be fresh")
	}
}

func TestNoFileMeansNoCacheAndNotFresh(t *testing.T) {
	got, fresh, err := (Store{Dir: t.TempDir()}).Load()
	if err != nil || got != nil || fresh {
		t.Errorf("Load = %v, fresh=%v, %v; want nil, false, nil", got, fresh, err)
	}
	if got, fresh, err := (Store{}).Load(); err != nil || got != nil || fresh {
		t.Errorf("no folder: %v, fresh=%v, %v", got, fresh, err)
	}
}

func TestSaveWithNoFolderErrors(t *testing.T) {
	if err := (Store{}).Save(sampleCategories()); err == nil {
		t.Error("expected an error saving with no folder set")
	}
}

func TestACorruptFileIsTreatedAsNoCacheRatherThanFailing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "browse_categories.jsonc"), []byte("not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, fresh, err := (Store{Dir: dir}).Load()
	if err != nil || got != nil || fresh {
		t.Errorf("corrupt file: got %v, fresh=%v, err=%v; want nil, false, nil", got, fresh, err)
	}
}

// TestACacheOlderThanStaleAfterIsNotFresh proves staleness is judged by real elapsed time, not
// just "does a file exist" - written directly via this package's own cachedFile/cachedCategory
// (rather than through Save, which always stamps CachedAt as now) so the file's age can be
// backdated precisely, the same idea internal/loverslab's own client_test.go uses to test its
// pageCacheTTL without actually sleeping in a test.
func TestACacheOlderThanStaleAfterIsNotFresh(t *testing.T) {
	dir := t.TempDir()
	cats := sampleCategories()
	cached := make([]cachedCategory, len(cats))
	for i, c := range cats {
		cached[i] = cachedCategory{ID: c.ID, Name: c.Name, URL: c.URL, Files: c.Files, Depth: c.Depth}
	}
	body, err := json.Marshal(cachedFile{Categories: cached, CachedAt: time.Now().Add(-StaleAfter - time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "browse_categories.jsonc"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	got, fresh, err := (Store{Dir: dir}).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if fresh {
		t.Error("a cache older than StaleAfter should not be fresh")
	}
	if !reflect.DeepEqual(got, cats) {
		t.Errorf("a stale cache should still return its categories as a fallback, got %+v", got)
	}
}

func TestTheFileIsCommentedJSONC(t *testing.T) {
	dir := t.TempDir()
	if err := (Store{Dir: dir}).Save(sampleCategories()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "browse_categories.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "// Parallax Mod Manager") {
		t.Errorf("the file does not explain itself:\n%s", text)
	}
	if strings.Contains(text, "—") {
		t.Error("em dash in the generated file")
	}
}
