package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func TestListModFilesListsRealFilesAndFolders(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)
	// writeMod already wrote mod_a/common/x.txt - add another nested file.
	writeFile(t, modDir, filepath.Join("mod_a", "gfx", "icon.dds"), "fake-binary-data")

	files, err := ListModFiles(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ListModFiles: %v", err)
	}

	byPath := map[string]FileEntry{}
	for _, e := range files.Entries {
		byPath[e.RelPath] = e
	}

	common, ok := byPath["common"]
	if !ok || !common.IsDir {
		t.Errorf("expected a dir entry for %q, got %+v (ok=%v)", "common", common, ok)
	}
	x, ok := byPath["common/x.txt"]
	if !ok || x.IsDir {
		t.Errorf("expected a file entry for %q, got %+v (ok=%v)", "common/x.txt", x, ok)
	}
	icon, ok := byPath["gfx/icon.dds"]
	if !ok || icon.IsDir || icon.Size != int64(len("fake-binary-data")) {
		t.Errorf("expected a file entry for %q with size %d, got %+v (ok=%v)", "gfx/icon.dds", len("fake-binary-data"), icon, ok)
	}

	wantTotal := int64(len("thing = { cost = 1 }") + len("fake-binary-data"))
	if files.TotalSize != wantTotal {
		t.Errorf("TotalSize = %d, want %d", files.TotalSize, wantTotal)
	}
	if files.Truncated {
		t.Error("Truncated = true, want false for a small mod")
	}

	wantModTime := time.Now()
	if err := os.Chtimes(filepath.Join(modDir, "mod_a", "gfx", "icon.dds"), wantModTime, wantModTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	files, err = ListModFiles(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ListModFiles (second run): %v", err)
	}
	if files.LastModified != wantModTime.Unix() {
		t.Errorf("LastModified = %d, want %d", files.LastModified, wantModTime.Unix())
	}
}

func TestListModFilesTruncatesHugeMods(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "big_mod.mod", `name = "Big Mod"
path = "big_mod"
`)
	for i := 0; i < maxModFileEntries+50; i++ {
		writeFile(t, modDir, filepath.Join("big_mod", "common", string(rune('a'+i%26))+string(rune('0'+i/26))+".txt"), "x")
	}

	files, err := ListModFiles(context.Background(), testGameConfig(), Options{ModDir: modDir}, "big_mod")
	if err != nil {
		t.Fatalf("ListModFiles: %v", err)
	}
	if !files.Truncated {
		t.Error("expected Truncated = true for a mod with more than maxModFileEntries files")
	}
	if len(files.Entries) > maxModFileEntries {
		t.Errorf("len(Entries) = %d, want at most %d", len(files.Entries), maxModFileEntries)
	}
	// TotalSize must still reflect everything, not just the capped list.
	if files.TotalSize != int64(maxModFileEntries+50) {
		t.Errorf("TotalSize = %d, want %d (every file is 1 byte)", files.TotalSize, maxModFileEntries+50)
	}
}

// TestListModFilesEmptyContentDirReturnsRealEmptySlice pins a real bug: a
// mod whose content folder has no files at all left ModFiles.Entries at
// Go's nil-slice zero value, which encoding/json marshals as JSON null -
// the generated TS binding assigns that straight through, so the frontend's
// Entries (typed FileEntry[]) would actually be null, crashing the file
// tab's own "this mod's content folder is empty" check (files.Entries.length)
// before it could ever show that message.
func TestListModFilesEmptyContentDirReturnsRealEmptySlice(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "empty_mod.mod", `name = "Empty Mod"
path = "empty_mod"
`)
	if err := os.MkdirAll(filepath.Join(modDir, "empty_mod"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	files, err := ListModFiles(context.Background(), testGameConfig(), Options{ModDir: modDir}, "empty_mod")
	if err != nil {
		t.Fatalf("ListModFiles: %v", err)
	}
	if files.Entries == nil {
		t.Error("Entries is nil, want a real empty slice (marshals as JSON null, not [])")
	}
	if len(files.Entries) != 0 {
		t.Errorf("Entries = %+v, want empty", files.Entries)
	}
}

// TestListModFilesReportsFriendlyErrorForMissingContent pins a real bug: a
// mod whose descriptor points at a content path that doesn't exist (a
// stale path, moved/renamed folder, or a drive that isn't connected right
// now - a real case hit on a real machine) used to surface only once
// something actually tried to read it, as a raw filesystem error like
// "lstat ...: no such file or directory" - confusing and leaking an
// internal path-resolution detail straight into the UI. findMod now checks
// mod.Mod.ContentMissing (computed by scan.Scan) first and returns a
// clear, human-readable message instead.
func TestListModFilesReportsFriendlyErrorForMissingContent(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "stale_mod.mod", `name = "Stale Mod"
path = "/this/path/does/not/exist/stale_mod"
`)

	_, err := ListModFiles(context.Background(), testGameConfig(), Options{ModDir: modDir}, "stale_mod")
	if err == nil {
		t.Fatal("expected an error for a mod with missing content")
	}
	msg := err.Error()
	if strings.Contains(msg, "lstat") || strings.Contains(msg, "no such file") {
		t.Errorf("error = %q, want a human-readable message, not a raw filesystem error", msg)
	}
	if strings.Contains(msg, "/this/path/does/not/exist") {
		t.Errorf("error = %q, want it to omit the raw absolute path - not useful to a user", msg)
	}
	if !strings.Contains(msg, "Stale Mod") {
		t.Errorf("error = %q, want it to name the mod", msg)
	}
}

func TestListModFilesUnknownModErrors(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)

	if _, err := ListModFiles(context.Background(), testGameConfig(), Options{ModDir: modDir}, "does_not_exist"); err == nil {
		t.Fatal("expected an error for an unknown mod ID")
	}
}

func TestModSizesSumsEveryModsRealContent(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`) // "thing = { cost = 1 }" -> common/x.txt
	writeFile(t, modDir, filepath.Join("mod_a", "gfx", "icon.dds"), "fake-binary-data")
	writeMod(t, modDir, "mod_b", "Mod B", `other = { cost = 2 }`)

	sizes, err := ModSizes(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("ModSizes: %v", err)
	}
	wantA := int64(len("thing = { cost = 1 }") + len("fake-binary-data"))
	wantB := int64(len("other = { cost = 2 }"))
	if sizes["mod_a"] != wantA {
		t.Errorf("mod_a size = %d, want %d", sizes["mod_a"], wantA)
	}
	if sizes["mod_b"] != wantB {
		t.Errorf("mod_b size = %d, want %d", sizes["mod_b"], wantB)
	}
}

func TestModSizesEmptyModDirReturnsEmptyMap(t *testing.T) {
	sizes, err := ModSizes(context.Background(), testGameConfig(), Options{ModDir: t.TempDir()})
	if err != nil {
		t.Fatalf("ModSizes: %v", err)
	}
	if len(sizes) != 0 {
		t.Errorf("expected an empty map, got %+v", sizes)
	}
}

func TestReadModFileReturnsRealContent(t *testing.T) {
	modDir := t.TempDir()
	before := time.Now().Add(-time.Second)
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)

	got, err := ReadModFile(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a", "common/x.txt")
	if err != nil {
		t.Fatalf("ReadModFile: %v", err)
	}
	if got.Content != `thing = { cost = 1 }` {
		t.Errorf("Content = %q, want %q", got.Content, `thing = { cost = 1 }`)
	}
	if got.ModifiedAt < before.Unix() {
		t.Errorf("ModifiedAt = %d, want a real mtime no earlier than %d", got.ModifiedAt, before.Unix())
	}
}

func TestReadModFileRejectsPathEscapingContentDir(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)
	// A real secret living next to (not inside) mod_a's own content dir.
	if err := os.WriteFile(filepath.Join(modDir, "secret.txt"), []byte("top secret"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ReadModFile(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a", "../secret.txt")
	if err == nil {
		t.Fatal("expected an error for a relPath escaping the mod's content directory")
	}
}

func TestReadModFileRejectsDirectory(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)

	_, err := ReadModFile(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a", "common")
	if err == nil {
		t.Fatal("expected an error when relPath names a directory")
	}
}

func TestReadModFileRejectsOversizedFile(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)
	big := make([]byte, maxReadModFileSize+1)
	writeFile(t, modDir, filepath.Join("mod_a", "gfx", "huge.txt"), string(big))

	_, err := ReadModFile(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a", "gfx/huge.txt")
	if err == nil {
		t.Fatal("expected an error for a file over maxReadModFileSize")
	}
}

func TestReadModFileUsesContentPathsRecordedByLoadGame(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)
	paths := &ContentPathCache{}
	opts := Options{CacheDir: t.TempDir(), ModDir: modDir, ContentPaths: paths}

	if _, err := LoadGame(context.Background(), testGameConfig(), opts); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	// With its descriptor gone, a fresh scan can no longer find mod_a at
	// all - so a successful read below proves ReadModFile took the path
	// LoadGame recorded instead of scanning again.
	if err := os.Remove(filepath.Join(modDir, "mod_a.mod")); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	got, err := ReadModFile(context.Background(), testGameConfig(), opts, "mod_a", "common/x.txt")
	if err != nil {
		t.Fatalf("ReadModFile: %v", err)
	}
	if got.Content != `thing = { cost = 1 }` {
		t.Errorf("Content = %q, want %q", got.Content, `thing = { cost = 1 }`)
	}
}

func TestReadModFileRescansWhenRecordedPathIsStale(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)
	paths := &ContentPathCache{}
	paths.remember("test-game", "mod_a", filepath.Join(modDir, "moved_away"))
	opts := Options{ModDir: modDir, ContentPaths: paths}

	got, err := ReadModFile(context.Background(), testGameConfig(), opts, "mod_a", "common/x.txt")
	if err != nil {
		t.Fatalf("ReadModFile: %v", err)
	}
	if got.Content != `thing = { cost = 1 }` {
		t.Errorf("Content = %q, want %q", got.Content, `thing = { cost = 1 }`)
	}
	if dir, ok := paths.lookup("test-game", "mod_a"); !ok || dir != filepath.Join(modDir, "mod_a") {
		t.Errorf("after a rescan the cache should hold the real path, got %q (ok=%v)", dir, ok)
	}
}

func TestReadModFileStillErrorsForUnknownMod(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)
	opts := Options{ModDir: modDir, ContentPaths: &ContentPathCache{}}

	if _, err := ReadModFile(context.Background(), testGameConfig(), opts, "nope", "common/x.txt"); err == nil {
		t.Fatal("expected an error for a mod that isn't in the scan or the cache")
	}
}

func TestContentPathCacheStoreReplacesAndSkipsMissingContent(t *testing.T) {
	dir := t.TempDir()
	paths := &ContentPathCache{}
	paths.Store("g", []mod.Mod{
		{ID: "present", ContentPath: dir},
		{ID: "missing", ContentPath: dir, ContentMissing: true},
	})
	if _, ok := paths.lookup("g", "present"); !ok {
		t.Error("present mod should be remembered")
	}
	if _, ok := paths.lookup("g", "missing"); ok {
		t.Error("a mod with missing content should not be remembered")
	}

	paths.Store("g", []mod.Mod{{ID: "other", ContentPath: dir}})
	if _, ok := paths.lookup("g", "present"); ok {
		t.Error("Store should replace the game's previous entries")
	}

	var none *ContentPathCache
	none.Store("g", nil) // must not panic
	if _, ok := none.lookup("g", "x"); ok {
		t.Error("a nil cache should never report a hit")
	}
}

func TestModFolderPathReturnsRealContentPath(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = { cost = 1 }`)

	path, err := ModFolderPath(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ModFolderPath: %v", err)
	}
	want := filepath.Join(modDir, "mod_a")
	if path != want {
		t.Errorf("ModFolderPath = %q, want %q", path, want)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("expected %q to be a real, existing directory", path)
	}
}

func TestModFolderPathUnknownModErrors(t *testing.T) {
	modDir := t.TempDir()
	if _, err := ModFolderPath(context.Background(), testGameConfig(), Options{ModDir: modDir}, "does_not_exist"); err == nil {
		t.Fatal("expected an error for an unknown mod ID")
	}
}
