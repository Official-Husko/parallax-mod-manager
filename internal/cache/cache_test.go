package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

func statOf(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	return info
}

func TestLookupUnchangedStatIsHit(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info := statOf(t, filePath)

	c := newModCache("stellaris", "test_mod")
	c.Put("a.txt", FileRecord{
		Path: "a.txt", ModTimeUnixNano: info.ModTime().UnixNano(), Size: info.Size(),
		Hash: 12345, Definitions: []definition.Definition{{ID: "x"}},
	})

	rec, ok := c.Lookup("a.txt", info)
	if !ok {
		t.Fatal("expected a cache hit for unchanged (mtime,size)")
	}
	if rec.Hash != 12345 || len(rec.Definitions) != 1 {
		t.Errorf("Lookup returned unexpected record: %+v", rec)
	}
}

func TestLookupChangedSizeIsMiss(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(filePath, []byte("hello world, longer now"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info := statOf(t, filePath)

	c := newModCache("stellaris", "test_mod")
	c.Put("a.txt", FileRecord{Path: "a.txt", ModTimeUnixNano: info.ModTime().UnixNano(), Size: 5, Hash: 1})

	if _, ok := c.Lookup("a.txt", info); ok {
		t.Fatal("expected a cache miss when size differs from the recorded value")
	}
}

func TestLookupTouchedMtimeIsMiss(t *testing.T) {
	// This is the "touched but content unchanged" case: Lookup itself only
	// checks (mtime,size), so a touched mtime alone is a Lookup miss even
	// though a caller doing the full stat->hash->parse pipeline would then
	// re-hash and find the content identical (see internal/pipeline).
	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info := statOf(t, filePath)

	c := newModCache("stellaris", "test_mod")
	c.Put("a.txt", FileRecord{
		Path:            "a.txt",
		ModTimeUnixNano: info.ModTime().Add(-time.Hour).UnixNano(), // stale mtime on purpose
		Size:            info.Size(),
		Hash:            42,
	})

	if _, ok := c.Lookup("a.txt", info); ok {
		t.Fatal("expected a cache miss when the recorded mtime differs")
	}
}

func TestLookupUnknownFileIsMiss(t *testing.T) {
	c := newModCache("stellaris", "test_mod")
	dir := t.TempDir()
	filePath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := c.Lookup("a.txt", statOf(t, filePath)); ok {
		t.Fatal("expected a miss for a file never recorded")
	}
}

func TestFileStoreSaveThenLoadRoundTrips(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx := context.Background()

	c := newModCache("stellaris", "test_mod")
	c.Put("common/buildings/00_buildings.txt", FileRecord{
		Path: "common/buildings/00_buildings.txt", ModTimeUnixNano: 123, Size: 456, Hash: 789,
		Definitions: []definition.Definition{{Type: "common/buildings", ID: "some_building", Hash: 111}},
	})

	if err := store.Save(ctx, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load(ctx, "stellaris", "test_mod")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rec, ok := loaded.Files["common/buildings/00_buildings.txt"]
	if !ok {
		t.Fatalf("loaded cache missing expected file record: %+v", loaded)
	}
	if rec.Hash != 789 || len(rec.Definitions) != 1 || rec.Definitions[0].ID != "some_building" {
		t.Errorf("round-tripped record = %+v", rec)
	}
}

func TestFileStoreLoadMissingFileReturnsEmptyCache(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	c, err := store.Load(context.Background(), "stellaris", "never_saved_mod")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Files) != 0 {
		t.Errorf("expected an empty cache, got %+v", c)
	}
	if c.Version != FormatVersion {
		t.Errorf("Version = %d, want %d", c.Version, FormatVersion)
	}
}

func TestFileStoreLoadCorruptJSONFailsClosed(t *testing.T) {
	dir := t.TempDir()
	gameDir := filepath.Join(dir, "stellaris")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "broken_mod.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store := FileStore{Dir: dir}
	c, err := store.Load(context.Background(), "stellaris", "broken_mod")
	if err != nil {
		t.Fatalf("Load returned an error instead of failing closed: %v", err)
	}
	if len(c.Files) != 0 {
		t.Errorf("expected an empty fallback cache for corrupt JSON, got %+v", c)
	}
}

func TestFileStoreLoadWrongVersionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	gameDir := filepath.Join(dir, "stellaris")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Version 99999 doesn't exist yet - simulates a cache written by some
	// future, incompatible version of this program.
	content := `{"version":99999,"modId":"old_mod","gameKey":"stellaris","files":{"a.txt":{"path":"a.txt","hash":1}}}`
	if err := os.WriteFile(filepath.Join(gameDir, "old_mod.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store := FileStore{Dir: dir}
	c, err := store.Load(context.Background(), "stellaris", "old_mod")
	if err != nil {
		t.Fatalf("Load returned an error instead of failing closed: %v", err)
	}
	if len(c.Files) != 0 {
		t.Errorf("expected an empty fallback cache for a version mismatch, got %+v", c)
	}
}

func TestFileStoreSaveIsAtomicNoLeftoverTempFiles(t *testing.T) {
	dir := t.TempDir()
	store := FileStore{Dir: dir}
	c := newModCache("stellaris", "test_mod")
	if err := store.Save(context.Background(), c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "stellaris"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "test_mod.json" {
		t.Errorf("expected exactly one file test_mod.json, got %+v", entries)
	}
}

func TestFileStoreLoadRespectsContextCancellation(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Load(ctx, "stellaris", "any_mod"); err == nil {
		t.Fatal("expected Load to report the cancelled context")
	}
}
