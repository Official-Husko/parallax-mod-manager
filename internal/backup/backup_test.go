package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testGame = "01a0963a-c214-75a3-908d-1b76b91ea7bf"

func write(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// makeMod builds a small Workshop-style mod folder.
func makeMod(t *testing.T) (dir string, mtime time.Time) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "content", "2780180614")
	mtime = time.Unix(1730223116, 0)
	write(t, filepath.Join(dir, "descriptor.mod"), `name="OUTDATED Cross Border Trade"`, mtime)
	write(t, filepath.Join(dir, "common", "traits", "a.txt"), "trait = {}\n", mtime.Add(-time.Hour))
	write(t, filepath.Join(dir, "localisation", "english", "l_english.yml"), "\xef\xbb\xbfl_english:\n a: \"b\"\n", mtime.Add(-2*time.Hour))
	write(t, filepath.Join(dir, "gfx", "big.bin"), strings.Repeat("x", 3<<20), mtime.Add(-3*time.Hour))
	if err := os.MkdirAll(filepath.Join(dir, "events", "empty_folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, mtime
}

func src(dir string) Source {
	return Source{GameID: testGame, ModID: "ugc_2780180614", RemoteFileID: "2780180614", Name: "OUTDATED Cross Border Trade", Version: "1.0", ContentPath: dir, Reason: ReasonDeleted}
}

func sameTree(t *testing.T, a, b string) {
	t.Helper()
	list := func(root string) map[string]string {
		out := map[string]string{}
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			rel, _ := filepath.Rel(root, p)
			if d.IsDir() {
				out[rel+"/"] = ""
				return nil
			}
			data, _ := os.ReadFile(p)
			info, _ := d.Info()
			out[rel] = string(bytes.TrimSpace([]byte(info.ModTime().UTC().Format(time.RFC3339)))) + "|" + string(data)
			return nil
		})
		return out
	}
	la, lb := list(a), list(b)
	if len(la) != len(lb) {
		t.Fatalf("trees differ in size: %d vs %d entries", len(la), len(lb))
	}
	for k, v := range la {
		if lb[k] != v {
			t.Errorf("%s differs (content or modification time)", k)
		}
	}
}

func TestCopyMakesAFaithfulOneToOneCopy(t *testing.T) {
	dir, _ := makeMod(t)
	root := t.TempDir()

	res, err := Copy(context.Background(), root, src(dir), nil)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if res.Skipped || !res.Entry.Complete || res.Entry.Files != 4 {
		t.Errorf("result = %+v, want a complete, written copy of 4 files", res)
	}
	dest := FolderFor(root, testGame, "2780180614")
	sameTree(t, dir, dest)

	// Nothing but the mod's own files is in the copy's folder.
	if _, err := os.Stat(filepath.Join(dest, IndexFile)); err == nil {
		t.Error("the index leaked into the mod's folder")
	}
	if _, err := os.Stat(dest + ".partial"); err == nil {
		t.Error("a .partial folder was left behind")
	}

	entries := List(root, testGame)
	if len(entries) != 1 || entries[0].Name != "OUTDATED Cross Border Trade" || entries[0].Reason != ReasonDeleted || !entries[0].Complete {
		t.Errorf("index = %+v", entries)
	}
}

func TestCopyPreservesModesAndTimes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	dir, _ := makeMod(t)
	exe := filepath.Join(dir, "run.sh")
	write(t, exe, "#!/bin/sh\n", time.Unix(1600000000, 0))
	_ = os.Chmod(exe, 0o755)
	root := t.TempDir()
	if _, err := Copy(context.Background(), root, src(dir), nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(FolderFor(root, testGame, "2780180614"), "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 || info.ModTime().Unix() != 1600000000 {
		t.Errorf("mode = %v, mtime = %d, want 0755 and 1600000000", info.Mode().Perm(), info.ModTime().Unix())
	}
}

func TestCopyOfAnUnchangedModIsSkipped(t *testing.T) {
	dir, _ := makeMod(t)
	root := t.TempDir()
	first, _ := Copy(context.Background(), root, src(dir), nil)
	res, err := Copy(context.Background(), root, src(dir), nil)
	if err != nil || !res.Skipped {
		t.Fatalf("second Copy = %+v, %v, want skipped", res, err)
	}
	if res.Entry.BackedUpAt != first.Entry.BackedUpAt {
		t.Error("a skipped copy changed the recorded time")
	}
}

func TestCopyOfAChangedModReplacesTheOldCopy(t *testing.T) {
	dir, mtime := makeMod(t)
	root := t.TempDir()
	if _, err := Copy(context.Background(), root, src(dir), nil); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "common", "traits", "a.txt"), "trait = { changed = yes }\n", mtime.Add(time.Hour))
	write(t, filepath.Join(dir, "common", "traits", "new.txt"), "new = {}\n", mtime.Add(time.Hour))
	if err := os.Remove(filepath.Join(dir, "descriptor.mod")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "descriptor.mod"), `name="Renamed"`, mtime.Add(time.Hour))

	res, err := Copy(context.Background(), root, src(dir), nil)
	if err != nil || res.Skipped {
		t.Fatalf("Copy after a change = %+v, %v, want a fresh copy", res, err)
	}
	sameTree(t, dir, FolderFor(root, testGame, "2780180614"))
	if _, err := os.Stat(FolderFor(root, testGame, "2780180614") + ".old"); err == nil {
		t.Error("the .old folder was left behind")
	}
}

func TestCopyRefusesUnsafeIDsAndOverlap(t *testing.T) {
	dir, _ := makeMod(t)
	root := t.TempDir()
	for _, id := range []string{"", "..", "../../etc", "12/34", "abc", "1 2"} {
		s := src(dir)
		s.RemoteFileID = id
		if _, err := Copy(context.Background(), root, s, nil); err == nil {
			t.Errorf("Copy accepted the item id %q", id)
		}
	}
	for _, game := range []string{"", "..", "a/b", `a\b`} {
		s := src(dir)
		s.GameID = game
		if _, err := Copy(context.Background(), root, s, nil); err == nil {
			t.Errorf("Copy accepted the game id %q", game)
		}
	}
	// A backup folder inside the mod, or the mod inside the backup folder.
	if _, err := Copy(context.Background(), dir, src(dir), nil); err == nil {
		t.Error("Copy accepted the mod's own folder as the backup folder")
	}
	inside := filepath.Join(dir, "backups")
	if _, err := Copy(context.Background(), inside, src(dir), nil); err == nil {
		t.Error("Copy accepted a backup folder inside the mod")
	}
	if _, err := os.Stat(inside); err == nil {
		t.Error("a refused copy still created folders inside the mod")
	}
}

func TestCopyOfAMissingSourceIsErrSourceGone(t *testing.T) {
	_, err := Copy(context.Background(), t.TempDir(), src(filepath.Join(t.TempDir(), "nope")), nil)
	if !errors.Is(err, ErrSourceGone) {
		t.Errorf("err = %v, want ErrSourceGone", err)
	}
	if _, err := Copy(context.Background(), "", src(t.TempDir()), nil); err == nil {
		t.Error("Copy with no root succeeded")
	}
}

func TestCopyRefusesWhenTheDiskIsFull(t *testing.T) {
	dir, _ := makeMod(t)
	root := t.TempDir()
	old := freeBytes
	freeBytes = func(string) (uint64, bool) { return 1 << 20, true }
	defer func() { freeBytes = old }()

	_, err := Copy(context.Background(), root, src(dir), nil)
	if !errors.Is(err, ErrNoSpace) {
		t.Fatalf("err = %v, want ErrNoSpace", err)
	}
	if _, statErr := os.Stat(FolderFor(root, testGame, "2780180614")); statErr == nil {
		t.Error("a refused copy left a folder behind")
	}
}

func TestCancelledCopyLeavesTheOldCopyAndNoPartial(t *testing.T) {
	dir, mtime := makeMod(t)
	root := t.TempDir()
	if _, err := Copy(context.Background(), root, src(dir), nil); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "common", "traits", "a.txt"), "changed\n", mtime.Add(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	_, err := Copy(ctx, root, src(dir), func(done, total int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	dest := FolderFor(root, testGame, "2780180614")
	if _, err := os.Stat(dest + ".partial"); err == nil {
		t.Error("a cancelled copy left a .partial folder")
	}
	data, _ := os.ReadFile(filepath.Join(dest, "common", "traits", "a.txt"))
	if string(data) != "trait = {}\n" {
		t.Errorf("the earlier copy was damaged: %q", data)
	}
}

func TestFilesVanishingMidCopyGiveAMarkedIncompleteCopy(t *testing.T) {
	dir, _ := makeMod(t)
	root := t.TempDir()
	// Steam removes the mod as it is copied: the first file read deletes the rest.
	first := true
	progress := func(done, total int64) {
		if first && done > 0 {
			first = false
			_ = os.RemoveAll(filepath.Join(dir, "gfx"))
			_ = os.RemoveAll(filepath.Join(dir, "localisation"))
		}
	}
	res, err := Copy(context.Background(), root, src(dir), progress)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if res.Entry.Complete || res.Entry.Missing == 0 {
		t.Errorf("entry = %+v, want it marked incomplete with missing files", res.Entry)
	}
	if _, err := os.Stat(filepath.Join(FolderFor(root, testGame, "2780180614"), "descriptor.mod")); err != nil {
		t.Error("the files that could be saved were not kept")
	}
}

func TestIncompleteCopyNeverReplacesACompleteOne(t *testing.T) {
	dir, mtime := makeMod(t)
	root := t.TempDir()
	if _, err := Copy(context.Background(), root, src(dir), nil); err != nil {
		t.Fatal(err)
	}
	// The mod changes, then loses files while being copied again.
	write(t, filepath.Join(dir, "common", "traits", "a.txt"), "changed\n", mtime.Add(time.Hour))
	first := true
	_, err := Copy(context.Background(), root, src(dir), func(done, total int64) {
		if first && done > 0 {
			first = false
			_ = os.RemoveAll(filepath.Join(dir, "gfx"))
		}
	})
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v, want ErrIncomplete", err)
	}
	dest := FolderFor(root, testGame, "2780180614")
	if data, err := os.ReadFile(filepath.Join(dest, "gfx", "big.bin")); err != nil || len(data) != 3<<20 {
		t.Error("the complete earlier copy was replaced by a partial one")
	}
	if e, ok := Lookup(root, testGame, "2780180614"); !ok || !e.Complete {
		t.Errorf("the index no longer describes the complete copy: %+v", e)
	}
}

func TestProgressReachesTheTotal(t *testing.T) {
	dir, _ := makeMod(t)
	var last, total int64
	_, err := Copy(context.Background(), t.TempDir(), src(dir), func(done, tot int64) { last, total = done, tot })
	if err != nil {
		t.Fatal(err)
	}
	if last != total || total == 0 {
		t.Errorf("progress ended at %d of %d", last, total)
	}
}

func TestSymlinksAreNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir, mtime := makeMod(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	write(t, outside, "do not copy", mtime)
	if err := os.Symlink(outside, filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := Copy(context.Background(), root, src(dir), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(FolderFor(root, testGame, "2780180614"), "link.txt")); err == nil {
		t.Error("a symlink was followed or copied")
	}
}

func TestListSkipsCopiesDeletedByHandAndSortsNewestFirst(t *testing.T) {
	root := t.TempDir()
	dir, _ := makeMod(t)
	other := filepath.Join(t.TempDir(), "111")
	write(t, filepath.Join(other, "descriptor.mod"), `name="Other"`, time.Unix(1, 0))

	if _, err := Copy(context.Background(), root, src(dir), nil); err != nil {
		t.Fatal(err)
	}
	s2 := src(other)
	s2.RemoteFileID, s2.ModID, s2.Name = "111", "ugc_111", "Other"
	if _, err := Copy(context.Background(), root, s2, nil); err != nil {
		t.Fatal(err)
	}
	if got := List(root, testGame); len(got) != 2 {
		t.Fatalf("List = %+v, want 2", got)
	}
	if err := os.RemoveAll(FolderFor(root, testGame, "111")); err != nil {
		t.Fatal(err)
	}
	got := List(root, testGame)
	if len(got) != 1 || got[0].RemoteFileID != "2780180614" {
		t.Errorf("List after deleting one by hand = %+v", got)
	}
	if List(root, "../etc") != nil || List("", testGame) != nil {
		t.Error("List accepted an unsafe game id or no root")
	}
}

func TestIndexFileIsJSONCWithAnExplanation(t *testing.T) {
	dir, _ := makeMod(t)
	root := t.TempDir()
	if _, err := Copy(context.Background(), root, src(dir), nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, testGame, IndexFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "// Parallax Mod Manager") || strings.Contains(string(data), "—") {
		t.Errorf("index header = %.80q", data)
	}
	// A corrupt index is an empty one, not an error: the copies are still there.
	_ = os.WriteFile(filepath.Join(root, testGame, IndexFile), []byte("{ nope"), 0o644)
	if got := List(root, testGame); len(got) != 0 {
		t.Errorf("List with a corrupt index = %+v", got)
	}
}
