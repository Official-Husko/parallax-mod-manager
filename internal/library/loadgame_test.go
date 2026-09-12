package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func testGameConfig() game.GameConfig {
	return game.GameConfig{
		ID:             "test-game",
		DisplayName:    "Test Game",
		DescriptorType: mod.DescriptorClassic,
		ScanFolders:    []string{"common"},
	}
}

func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// writeMod creates one classic-format mod under modDir: its descriptor (in
// modDir itself, per scan's flat-descriptor convention) plus its content
// under its own subfolder.
func writeMod(t *testing.T, modDir, id, name, script string) {
	t.Helper()
	writeFile(t, modDir, id+".mod", `name = "`+name+`"
path = "`+id+`"
version = "1.0"
`)
	writeFile(t, modDir, filepath.Join(id, "common", "x.txt"), script)
}

func TestLoadGameCleanScanNoConflicts(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing_b = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 2 {
		t.Fatalf("expected 2 mods, got %d: %+v", len(summary.Mods), summary.Mods)
	}
	if len(summary.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %+v", summary.Conflicts)
	}
	if len(summary.Errors) != 0 {
		t.Errorf("expected no errors, got %+v", summary.Errors)
	}
	if summary.Game.ID != "test-game" {
		t.Errorf("Game.ID = %q, want test-game", summary.Game.ID)
	}
}

func TestLoadGameDetectsGenuineConflict(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(summary.Conflicts), summary.Conflicts)
	}
	c := summary.Conflicts[0]
	if c.ID != "shared_thing" {
		t.Errorf("Conflict.ID = %q, want shared_thing", c.ID)
	}
	if len(c.Candidates) != 2 || c.Candidates[0] != "Mod A" || c.Candidates[1] != "Mod B" {
		t.Errorf("Candidates = %v, want [Mod A, Mod B] (mod display names, not IDs)", c.Candidates)
	}
}

func TestLoadGameMalformedModRecordedAsErrorNotFatal(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "good_mod", "Good Mod", `thing = { cost = 1 }`)
	// A descriptor that fails to parse.
	writeFile(t, modDir, "bad_mod.mod", `name = "unterminated`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 1 || summary.Mods[0].ID != "good_mod" {
		t.Errorf("Mods = %+v, want exactly [good_mod] (the malformed descriptor is a scan error, not a listed mod)", summary.Mods)
	}
	if len(summary.Errors) != 1 {
		t.Errorf("Errors = %v, want exactly 1 (the malformed descriptor)", summary.Errors)
	}
}

func TestLoadGameDefaultOrderEnablesEveryMod(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing_b = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	for _, m := range summary.Mods {
		if !m.Enabled {
			t.Errorf("mod %q Enabled = false, want true (nil Order enables everything)", m.ID)
		}
	}
}

func TestLoadGameExplicitOrderDisablesAbsentMods(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	// mod_b's content is malformed - if LoadGame parses it anyway, that
	// parse failure would show up in summary.Errors, proving the "disabled
	// mods are never parsed" guarantee actually holds, not just that
	// they're excluded from conflict detection.
	writeMod(t, modDir, "mod_b", "Mod B", `this is not valid { clausewitz syntax`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a"}, // mod_b is absent - disabled
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}

	if len(summary.Mods) != 2 {
		t.Fatalf("expected both mods listed regardless of enabled state, got %+v", summary.Mods)
	}
	byID := map[string]ModSummary{}
	for _, m := range summary.Mods {
		byID[m.ID] = m
	}
	if !byID["mod_a"].Enabled {
		t.Error("mod_a.Enabled = false, want true (present in Order)")
	}
	if byID["mod_b"].Enabled {
		t.Error("mod_b.Enabled = true, want false (absent from Order)")
	}
	if len(summary.Errors) != 0 {
		t.Errorf("Errors = %v, want none - mod_b's malformed content must never be parsed while disabled", summary.Errors)
	}
}

func TestLoadGameDisabledModExcludedFromConflicts(t *testing.T) {
	modDir := t.TempDir()
	// Both mods define the same key with different content - normally a
	// conflict, but mod_b is disabled via Order, so it must not appear.
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a"},
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Conflicts) != 0 {
		t.Errorf("expected no conflicts (mod_b disabled), got %+v", summary.Conflicts)
	}
}

func TestLoadGameEmptyModDirNoError(t *testing.T) {
	summary, err := LoadGame(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if len(summary.Mods) != 0 {
		t.Errorf("expected no mods, got %+v", summary.Mods)
	}
}
