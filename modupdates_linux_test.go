package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modupdates"
)

// modUpdatesApp is an App with one game whose mod folder is a temp folder and
// whose mod snapshots go to snapshotDir. Building a second one over the same
// dirs is a restart of the app.
func modUpdatesApp(snapshotDir string) *App {
	cfg := game.GameConfig{
		ID:             "test-game",
		DisplayName:    "Test Game",
		FolderName:     "TestGame",
		DescriptorType: mod.DescriptorClassic,
		ScanFolders:    []string{"common"},
	}
	a := &App{ctx: context.Background(), registry: game.NewRegistry([]game.GameConfig{cfg})}
	a.modUpdates.Store = modupdates.Store{Dir: snapshotDir}
	return a
}

func writeTestMod(t *testing.T, modDir, id, name, version, body string) {
	t.Helper()
	write := func(rel, content string) {
		full := filepath.Join(modDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(id+".mod", "name = \""+name+"\"\npath = \""+id+"\"\nversion = \""+version+"\"\n")
	write(filepath.Join(id, "common", "x.txt"), body)
}

func changeFor(t *testing.T, r modupdates.Report, id string) modupdates.Change {
	t.Helper()
	for _, c := range r.Changes {
		if c.ModID == id {
			return c
		}
	}
	t.Fatalf("no change for %q in %+v", id, r.Changes)
	return modupdates.Change{}
}

func TestCheckModUpdatesReportsWhatChangedSinceTheLastStartup(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	modDir := filepath.Join(dataHome, "Paradox Interactive", "TestGame", "mod")
	snapshotDir := t.TempDir()

	writeTestMod(t, modDir, "mod_a", "Mod A", "1.0", "a = { x = 1 }")
	writeTestMod(t, modDir, "mod_b", "Mod B", "1.0", "b = { x = 1 }")
	writeTestMod(t, modDir, "mod_c", "Mod C", "1.0", "c = { x = 1 }")
	writeTestMod(t, modDir, "mod_same", "Untouched", "1.0", "s = { x = 1 }")
	// The generated patch is rewritten by the app itself; it must never show up.
	writeTestMod(t, modDir, library.PatchModID, "Parallax Patch", "1.0", "patch = { gen = 1 }")

	// First run: nothing to compare with, but a snapshot is left behind.
	first, err := modUpdatesApp(snapshotDir).CheckModUpdates("test-game", false)
	if err != nil {
		t.Fatalf("first CheckModUpdates: %v", err)
	}
	if first.BaselineAt != 0 || len(first.Changes) != 0 || first.ModsChecked != 4 || !first.WorkshopChecked {
		t.Fatalf("first run = %+v, want no baseline, no changes, 4 mods (the patch left out)", first)
	}
	raw, err := os.ReadFile(filepath.Join(snapshotDir, "test-game.jsonc"))
	if err != nil {
		t.Fatalf("no snapshot written: %v", err)
	}
	if !strings.HasPrefix(string(raw), "// Parallax Mod Manager") {
		t.Errorf("snapshot has no explanatory comment:\n%s", raw)
	}

	// While the app is closed: A gets a new version, B gets a new file, C is
	// uninstalled, and the patch is regenerated.
	writeTestMod(t, modDir, "mod_a", "Mod A", "2.0", "a = { x = 1 }")
	writeTestMod(t, modDir, "mod_b", "Mod B", "1.0", "b = { x = 1 }")
	if err := os.WriteFile(filepath.Join(modDir, "mod_b", "common", "extra.txt"), []byte("extra = { y = 2 }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(modDir, "mod_c.mod")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(modDir, "mod_c")); err != nil {
		t.Fatal(err)
	}
	writeTestMod(t, modDir, library.PatchModID, "Parallax Patch", "1.0", "patch = { gen = 2, more = yes }")

	// Second run (a restart).
	restarted := modUpdatesApp(snapshotDir)
	second, err := restarted.CheckModUpdates("test-game", false)
	if err != nil {
		t.Fatalf("second CheckModUpdates: %v", err)
	}
	if second.BaselineAt == 0 {
		t.Fatalf("second run has no baseline: %+v", second)
	}
	if len(second.Changes) != 3 {
		t.Fatalf("changes = %+v, want exactly A updated, B changed, C removed", second.Changes)
	}
	if a := changeFor(t, second, "mod_a"); a.Kind != modupdates.KindUpdated || a.FromVersion != "1.0" || a.ToVersion != "2.0" || !a.New {
		t.Errorf("mod_a = %+v, want updated 1.0 -> 2.0", a)
	}
	if b := changeFor(t, second, "mod_b"); b.Kind != modupdates.KindChanged || b.FilesDelta != 1 || b.SizeDelta <= 0 {
		t.Errorf("mod_b = %+v, want changed with one file more", b)
	}
	if c := changeFor(t, second, "mod_c"); c.Kind != modupdates.KindRemoved || c.Name != "Mod C" {
		t.Errorf("mod_c = %+v, want removed", c)
	}

	// Checking again in the same run still reports everything since startup.
	again, _ := restarted.CheckModUpdates("test-game", false)
	if len(again.Changes) != 3 {
		t.Errorf("a second check in the same run lost changes: %+v", again.Changes)
	}

	// Marking them seen clears the list, and a later check stays clear.
	if marked := restarted.MarkModUpdatesSeen("test-game"); len(marked.Changes) != 0 {
		t.Errorf("after marking seen = %+v, want none", marked.Changes)
	}
	if after, _ := restarted.CheckModUpdates("test-game", false); len(after.Changes) != 0 {
		t.Errorf("a check after marking seen = %+v, want none", after.Changes)
	}

	// The next startup does not repeat what this one already showed.
	third, _ := modUpdatesApp(snapshotDir).CheckModUpdates("test-game", false)
	if len(third.Changes) != 0 {
		t.Errorf("third run = %+v, want nothing new", third.Changes)
	}
}

func TestCheckModUpdatesUnknownGame(t *testing.T) {
	a := modUpdatesApp(t.TempDir())
	if _, err := a.CheckModUpdates("nope", false); err == nil {
		t.Error("expected an error for an unknown game")
	}
}
