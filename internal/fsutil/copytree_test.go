package fsutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCopyTreeCopiesNestedFoldersFaithfully(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	writeFile(t, filepath.Join(src, "descriptor.mod"), `name="Test"`)
	writeFile(t, filepath.Join(src, "common", "buildings", "a.txt"), "a")
	writeFile(t, filepath.Join(src, "localisation", "english", "b.yml"), "b")

	stats, err := CopyTree(context.Background(), src, dst, 0, nil)
	if err != nil {
		t.Fatalf("CopyTree: %v", err)
	}
	if stats.Files != 3 {
		t.Fatalf("Files = %d, want 3", stats.Files)
	}
	if stats.Missing != 0 || stats.Skipped != 0 {
		t.Fatalf("Missing = %d, Skipped = %d, want 0/0", stats.Missing, stats.Skipped)
	}
	for _, rel := range []string{"descriptor.mod", filepath.Join("common", "buildings", "a.txt"), filepath.Join("localisation", "english", "b.yml")} {
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Fatalf("reading copied %s: %v", rel, err)
		}
		want, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s: got %q, want %q", rel, got, want)
		}
	}
}

func TestCopyTreeKeepsPermissionsAndModTime(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	path := filepath.Join(src, "file.txt")
	writeFile(t, path, "x")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	if _, err := CopyTree(context.Background(), src, dst, 0, nil); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}
	info, err := os.Stat(filepath.Join(dst, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("perm = %v, want 0640", info.Mode().Perm())
	}
	if !info.ModTime().Equal(mtime) {
		t.Errorf("modtime = %v, want %v", info.ModTime(), mtime)
	}
}

func TestCopyTreeReportsProgressWithFileNames(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	writeFile(t, filepath.Join(src, "a.txt"), "aa")
	writeFile(t, filepath.Join(src, "b.txt"), "bb")

	var calls []string
	var lastDone, total int64
	_, err := CopyTree(context.Background(), src, dst, 4, func(file string, done, tot int64) {
		calls = append(calls, file)
		lastDone, total = done, tot
	})
	if err != nil {
		t.Fatalf("CopyTree: %v", err)
	}
	if len(calls) < 3 { // one "started" call plus one per file
		t.Fatalf("got %d progress calls, want at least 3: %v", len(calls), calls)
	}
	if calls[0] != "" {
		t.Errorf("first progress call's file = %q, want \"\" (the started signal)", calls[0])
	}
	if lastDone != 4 || total != 4 {
		t.Errorf("final progress = %d/%d, want 4/4", lastDone, total)
	}
}

func TestCopyTreeCancelStopsPromptly(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(src, "file"+string(rune('a'+i))+".txt"), "some content padded out a little")
	}
	ctx, cancel := context.WithCancel(context.Background())
	seen := 0
	_, err := CopyTree(ctx, src, dst, 0, func(file string, done, total int64) {
		seen++
		if seen == 3 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("CopyTree: want an error after cancelling, got nil")
	}
	if ctx.Err() == nil {
		t.Fatal("context was not cancelled")
	}
}

func TestCopyTreeMissingFileIsNotFatal(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	present := filepath.Join(src, "present.txt")
	gone := filepath.Join(src, "gone.txt")
	writeFile(t, present, "here")
	writeFile(t, gone, "briefly here")
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	stats, err := CopyTree(context.Background(), src, dst, 0, nil)
	if err != nil {
		t.Fatalf("CopyTree: %v", err)
	}
	if stats.Files != 1 {
		t.Errorf("Files = %d, want 1", stats.Files)
	}
}

func TestDirStatCountsFilesAndBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "12345")
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), "1234567890")

	files, bytes, err := DirStat(dir)
	if err != nil {
		t.Fatalf("DirStat: %v", err)
	}
	if files != 2 {
		t.Errorf("files = %d, want 2", files)
	}
	if bytes != 15 {
		t.Errorf("bytes = %d, want 15", bytes)
	}
}

func TestDirStatOnMissingPathIsAnError(t *testing.T) {
	_, _, err := DirStat(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("DirStat: want an error for a missing path, got nil")
	}
}
