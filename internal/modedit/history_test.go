package modedit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	mod, modDir, hist string
	store             Store
	tick              int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{mod: filepath.Join(root, "content"), modDir: filepath.Join(root, "user", "mod"), hist: filepath.Join(root, "settings", "history")}
	for _, d := range []string{f.mod, f.modDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.store = Store{Dir: f.hist, Roots: []string{f.mod, f.modDir}}
	// Each call gets a later moment, so saves sort in the order they were made.
	base := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	f.store.Now = func() time.Time { f.tick++; return base.Add(time.Duration(f.tick) * time.Second) }
	return f
}

func write(t *testing.T, path, body string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), perm); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(b)
}

func TestApplyWritesFilesAndKeepsTheOldOnes(t *testing.T) {
	f := newFixture(t)
	desc := filepath.Join(f.mod, "descriptor.mod")
	stubPath := filepath.Join(f.modDir, "a.mod")
	write(t, desc, "old descriptor", 0o644)
	write(t, stubPath, "old stub", 0o640)

	if _, err := f.store.Apply([]Write{{Path: desc, Data: []byte("new descriptor")}, {Path: stubPath, Data: []byte("new stub")}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if read(t, desc) != "new descriptor" || read(t, stubPath) != "new stub" {
		t.Error("the new contents were not written")
	}
	if info, _ := os.Stat(stubPath); info.Mode().Perm() != 0o640 {
		t.Errorf("permissions changed to %v, want 0640", info.Mode().Perm())
	}
	if n, when := f.store.History(); n != 1 || when.IsZero() {
		t.Errorf("History = %d, %v", n, when)
	}
	// Nothing but the wanted files is left in the mod folders.
	for _, dir := range []string{f.mod, f.modDir} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".tmp-") {
				t.Errorf("a temp file was left in %s: %s", dir, e.Name())
			}
		}
	}
	// The history holds the old bytes, outside the mod folders.
	backups := 0
	filepath.Walk(f.hist, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".bak") {
			backups++
			if b := read(t, p); b != "old descriptor" && b != "old stub" {
				t.Errorf("backup %s holds %q", p, b)
			}
		}
		return nil
	})
	if backups != 2 {
		t.Errorf("%d backups, want 2", backups)
	}
}

func TestUndoRestoresWhatTheLastSaveReplaced(t *testing.T) {
	f := newFixture(t)
	desc := filepath.Join(f.mod, "descriptor.mod")
	thumb := filepath.Join(f.mod, "thumbnail.png")
	write(t, desc, "v1", 0o644)

	// A save that changes a file and creates one.
	if _, err := f.store.Apply([]Write{{Path: desc, Data: []byte("v2")}, {Path: thumb, Data: []byte("png")}}); err != nil {
		t.Fatal(err)
	}
	restored, err := f.store.Undo()
	if err != nil || len(restored) != 2 {
		t.Fatalf("Undo = %v, %v", restored, err)
	}
	if read(t, desc) != "v1" {
		t.Errorf("descriptor = %q, want v1", read(t, desc))
	}
	if _, err := os.Stat(thumb); !os.IsNotExist(err) {
		t.Errorf("a file the save created should be removed by undo: %v", err)
	}
	if n, _ := f.store.History(); n != 0 {
		t.Errorf("History = %d after undoing the only save", n)
	}
	if _, err := f.store.Undo(); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("a second Undo = %v, want ErrNothingToUndo", err)
	}
}

func TestUndoGoesBackOneSaveAtATime(t *testing.T) {
	f := newFixture(t)
	p := filepath.Join(f.mod, "descriptor.mod")
	write(t, p, "v1", 0o644)
	for _, v := range []string{"v2", "v3", "v4"} {
		if _, err := f.store.Apply([]Write{{Path: p, Data: []byte(v)}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"v3", "v2", "v1"} {
		if _, err := f.store.Undo(); err != nil {
			t.Fatalf("Undo: %v", err)
		}
		if got := read(t, p); got != want {
			t.Errorf("after undo: %q, want %q", got, want)
		}
	}
}

// If something else changed the file, restoring the old copy would throw that change away.
func TestUndoRefusesAFileChangedSinceTheSave(t *testing.T) {
	f := newFixture(t)
	p := filepath.Join(f.mod, "descriptor.mod")
	other := filepath.Join(f.modDir, "a.mod")
	write(t, p, "v1", 0o644)
	write(t, other, "s1", 0o644)
	if _, err := f.store.Apply([]Write{{Path: p, Data: []byte("v2")}, {Path: other, Data: []byte("s2")}}); err != nil {
		t.Fatal(err)
	}
	write(t, other, "edited by hand afterwards", 0o644)

	if _, err := f.store.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("Undo = %v, want ErrChangedSince", err)
	}
	// Nothing at all was touched, not even the file that had not changed.
	if read(t, p) != "v2" || read(t, other) != "edited by hand afterwards" {
		t.Error("a refused undo changed files")
	}
	if n, _ := f.store.History(); n != 1 {
		t.Errorf("a refused undo used up the save: History = %d", n)
	}
}

func TestOnlyTheNewestSavesAreKept(t *testing.T) {
	f := newFixture(t)
	p := filepath.Join(f.mod, "descriptor.mod")
	write(t, p, "v0", 0o644)
	for i := 1; i <= KeptSaves+4; i++ {
		if _, err := f.store.Apply([]Write{{Path: p, Data: []byte("v" + string(rune('a'+i)))}}); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := f.store.History(); n != KeptSaves {
		t.Errorf("History = %d, want %d", n, KeptSaves)
	}
}

func TestWritesOutsideTheModsFoldersAreRefused(t *testing.T) {
	f := newFixture(t)
	outside := filepath.Join(t.TempDir(), "elsewhere.mod")
	if _, err := f.store.Apply([]Write{{Path: outside, Data: []byte("x")}}); err == nil {
		t.Error("a write outside the mod's folders should be refused")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Error("the file was written anyway")
	}
	// A path that only starts like a root is not inside it.
	lookalike := f.mod + "-sibling"
	os.MkdirAll(lookalike, 0o755)
	if _, err := f.store.Apply([]Write{{Path: filepath.Join(lookalike, "x.mod"), Data: []byte("x")}}); err == nil {
		t.Error("a sibling folder with the same prefix should be refused")
	}
	// Traversal is cleaned before the check.
	if _, err := f.store.Apply([]Write{{Path: filepath.Join(f.mod, "..", "escape.mod"), Data: []byte("x")}}); err == nil {
		t.Error("a path that climbs out of the folder should be refused")
	}
}

// A tampered record cannot make an undo write outside the mod's folders.
func TestUndoRefusesAPathOutsideTheRoots(t *testing.T) {
	f := newFixture(t)
	p := filepath.Join(f.mod, "descriptor.mod")
	write(t, p, "v1", 0o644)
	if _, err := f.store.Apply([]Write{{Path: p, Data: []byte("v2")}}); err != nil {
		t.Fatal(err)
	}
	names, _ := os.ReadDir(f.hist)
	manifestPath := filepath.Join(f.hist, names[0].Name(), "save.jsonc")
	body := read(t, manifestPath)
	write(t, manifestPath, strings.ReplaceAll(body, p, filepath.Join(t.TempDir(), "victim.mod")), 0o644)
	if _, err := f.store.Undo(); err == nil {
		t.Error("expected undo to refuse a path outside the roots")
	}
}

func TestAFailedSaveLeavesNoTrace(t *testing.T) {
	f := newFixture(t)
	good := filepath.Join(f.mod, "descriptor.mod")
	write(t, good, "v1", 0o644)
	// The second file's folder is a file, so writing there fails after the first was written.
	blocker := filepath.Join(f.mod, "blocker")
	write(t, blocker, "i am a file", 0o644)
	bad := filepath.Join(blocker, "thumbnail.png")

	if _, err := f.store.Apply([]Write{{Path: good, Data: []byte("v2")}, {Path: bad, Data: []byte("png")}}); err == nil {
		t.Fatal("expected the second write to fail")
	}
	if read(t, good) != "v1" {
		t.Errorf("the first file was left changed: %q", read(t, good))
	}
	if n, _ := f.store.History(); n != 0 {
		t.Errorf("a failed save left %d entries in the history", n)
	}
}

func TestManifestIsCommentedJSONC(t *testing.T) {
	f := newFixture(t)
	p := filepath.Join(f.mod, "descriptor.mod")
	write(t, p, "v1", 0o644)
	if _, err := f.store.Apply([]Write{{Path: p, Data: []byte("v2")}}); err != nil {
		t.Fatal(err)
	}
	names, _ := os.ReadDir(f.hist)
	body := read(t, filepath.Join(f.hist, names[0].Name(), "save.jsonc"))
	if !strings.HasPrefix(body, "// Parallax Mod Manager") || !strings.Contains(body, `"afterSha256"`) {
		t.Errorf("manifest:\n%s", body)
	}
	if strings.Contains(body, "—") {
		t.Error("em dash in the manifest")
	}
}

func TestNoHistoryFolderIsAnErrorNotASilentSkip(t *testing.T) {
	f := newFixture(t)
	f.store.Dir = ""
	p := filepath.Join(f.mod, "descriptor.mod")
	write(t, p, "v1", 0o644)
	if _, err := f.store.Apply([]Write{{Path: p, Data: []byte("v2")}}); err == nil {
		t.Error("saving with nowhere to keep the old version must fail rather than skip the safety net")
	}
	if read(t, p) != "v1" {
		t.Error("the file was written without a safety net")
	}
}
