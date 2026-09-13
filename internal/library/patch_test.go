package library

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func TestGeneratePatchWritesWinningContentVerbatim(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"}, // mod_b last - LIOS winner
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if !result.Written {
		t.Fatal("expected Written = true")
	}
	if result.PatchedKeys != 1 {
		t.Errorf("PatchedKeys = %d, want 1", result.PatchedKeys)
	}
	if result.ModID != patchModID {
		t.Errorf("ModID = %q, want %q", result.ModID, patchModID)
	}

	// The patch's stub descriptor must exist and be discoverable.
	descPath := filepath.Join(modDir, patchModID+".mod")
	if _, err := os.Stat(descPath); err != nil {
		t.Fatalf("expected a descriptor at %s: %v", descPath, err)
	}

	// The patched content file (grouped by Type, which for writeMod's
	// fixture is "common") must contain mod_b's exact source text -
	// byte-for-byte, not a reformatted/re-serialized version.
	contentPath := filepath.Join(modDir, patchModID, "common", patchModID+".txt")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `shared_thing = { cost = 2 }`) {
		t.Errorf("patch content = %q, want it to contain mod_b's exact source", data)
	}
	if strings.Contains(string(data), `cost = 1`) {
		t.Errorf("patch content = %q, should not contain the loser's (mod_a's) content", data)
	}
}

func TestGeneratePatchHonorsManualOverride(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir:  t.TempDir(),
		ModDir:    modDir,
		Order:     conflict.LoadOrder{"mod_a", "mod_b"}, // mod_b would automatically win (LIOS)
		Overrides: map[string]string{"common:shared_thing": "mod_a"},
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if !result.Written {
		t.Fatal("expected Written = true")
	}

	contentPath := filepath.Join(modDir, patchModID, "common", patchModID+".txt")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `shared_thing = { cost = 1 }`) {
		t.Errorf("patch content = %q, want the manually overridden mod_a's exact source", data)
	}
	if strings.Contains(string(data), `cost = 2`) {
		t.Errorf("patch content = %q, should not contain the automatic winner (mod_b) once overridden", data)
	}
}

func TestGeneratePatchNoConflictsWritesNothing(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `thing_b = { cost = 2 }`)

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if result.Written {
		t.Fatal("expected Written = false when nothing conflicts")
	}
	if _, err := os.Stat(filepath.Join(modDir, patchModID+".mod")); !os.IsNotExist(err) {
		t.Errorf("expected no patch descriptor to be written, stat err = %v", err)
	}
}

func TestGeneratePatchRegeneratesCleanlyDroppingStaleTypes(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	opts := Options{CacheDir: t.TempDir(), ModDir: modDir, Order: conflict.LoadOrder{"mod_a", "mod_b"}}
	first, err := GeneratePatch(context.Background(), testGameConfig(), opts)
	if err != nil {
		t.Fatalf("first GeneratePatch: %v", err)
	}
	if !first.Written {
		t.Fatal("expected the first generation to write a patch")
	}
	contentPath := filepath.Join(modDir, patchModID, "common", patchModID+".txt")
	if _, err := os.Stat(contentPath); err != nil {
		t.Fatalf("expected patch content to exist after first generation: %v", err)
	}

	// Remove mod_b - the conflict is gone, so a fresh generation must not
	// leave the old "common" override behind.
	if err := os.Remove(filepath.Join(modDir, "mod_b.mod")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(modDir, "mod_b")); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	second, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: opts.CacheDir, ModDir: modDir, Order: conflict.LoadOrder{"mod_a"},
	})
	if err != nil {
		t.Fatalf("second GeneratePatch: %v", err)
	}
	if second.Written {
		t.Fatal("expected the second generation to have nothing to patch")
	}
	if _, err := os.Stat(contentPath); !os.IsNotExist(err) {
		t.Errorf("expected the stale patch content to be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(modDir, patchModID+".mod")); !os.IsNotExist(err) {
		t.Errorf("expected the stale patch descriptor to be gone, stat err = %v", err)
	}
}

func TestGeneratePatchRejectsJSONDescriptorGames(t *testing.T) {
	modDir := t.TempDir()
	cfg := testGameConfig()
	cfg.DescriptorType = mod.DescriptorJSONv1

	_, err := GeneratePatch(context.Background(), cfg, Options{CacheDir: t.TempDir(), ModDir: modDir})
	if err == nil {
		t.Fatal("expected an error for a JSON-descriptor game")
	}
}

func TestGeneratePatchWritesRealLocalisationFile(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "mod_a.mod", `name = "Mod A"
path = "mod_a"
version = "1.0"
`)
	writeFile(t, modDir, filepath.Join("mod_a", "common", "l_english.yml"), "l_english:\n KEY:0 \"Hello\"\n")
	writeFile(t, modDir, "mod_b.mod", `name = "Mod B"
path = "mod_b"
version = "1.0"
`)
	writeFile(t, modDir, filepath.Join("mod_b", "common", "l_english.yml"), "l_english:\n KEY:0 \"Goodbye\"\n")

	result, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"}, // mod_b last - LIOS winner
	})
	if err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}
	if !result.Written {
		t.Fatal("expected Written = true - localisation conflicts are patchable")
	}
	if result.PatchedKeys != 1 {
		t.Errorf("PatchedKeys = %d, want 1", result.PatchedKeys)
	}

	// Real folder name Paradox actually uses (British spelling), a real
	// .yml extension, not the .txt script files get.
	contentPath := filepath.Join(modDir, patchModID, "localisation", "english", patchModID+".yml")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("expected the generated .yml to start with a UTF-8 BOM, like real Paradox locale files")
	}
	if !bytes.Contains(data, []byte("l_english:")) {
		t.Errorf("content = %q, want it to contain a real l_english: header", data)
	}
	if !bytes.Contains(data, []byte(`KEY:0 "Goodbye"`)) {
		t.Errorf("content = %q, want mod_b's exact winning entry", data)
	}
	if bytes.Contains(data, []byte("Hello")) {
		t.Errorf("content = %q, should not contain the loser's (mod_a's) entry", data)
	}

	// The written file must itself parse back as a valid locale file.
	cat, err := locale.Parse(data)
	if err != nil {
		t.Fatalf("locale.Parse(generated patch content): %v", err)
	}
	if cat.Language != "english" || len(cat.Entries) != 1 || cat.Entries[0].Value != "Goodbye" {
		t.Errorf("parsed patch content = %+v, want language=english with 1 entry \"Goodbye\"", cat)
	}
}

func TestGeneratePatchDescriptorPointsAtRealContentDir(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	if _, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	}); err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(modDir, patchModID+".mod"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	desc, err := mod.ParseDescriptor(data, mod.DescriptorClassic)
	if err != nil {
		t.Fatalf("ParseDescriptor: %v", err)
	}
	want := filepath.Join(modDir, patchModID)
	if desc.Path != want {
		t.Errorf("descriptor Path = %q, want %q", desc.Path, want)
	}

	// The written descriptor must itself be a real, valid scannable mod -
	// re-scanning modDir should find it.
	scanResult, err := scanTestDir(t, modDir)
	if err != nil {
		t.Fatalf("re-scan: %v", err)
	}
	found := false
	for _, m := range scanResult {
		if m.ID == patchModID {
			found = true
		}
	}
	if !found {
		t.Error("expected the generated patch to be discoverable as an ordinary mod on the next scan")
	}
}

// scanTestDir re-scans modDir via LoadGame (Order nil - every mod listed,
// none enabled) purely to confirm the generated patch is a real,
// discoverable mod, without depending on internal/scan directly.
func scanTestDir(t *testing.T, modDir string) ([]ModSummary, error) {
	t.Helper()
	summary, err := LoadGame(context.Background(), testGameConfig(), Options{CacheDir: t.TempDir(), ModDir: modDir})
	if err != nil {
		return nil, err
	}
	return summary.Mods, nil
}

func TestGeneratePatchNeverOverwritesUnrelatedFiles(t *testing.T) {
	// A sanity guard: generating a patch must never touch any file outside
	// its own zzzzz_parallax_patch.mod / zzzzz_parallax_patch/ names.
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `shared_thing = { cost = 1 }`)
	writeMod(t, modDir, "mod_b", "Mod B", `shared_thing = { cost = 2 }`)

	beforeA, err := os.ReadFile(filepath.Join(modDir, "mod_a.mod"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if _, err := GeneratePatch(context.Background(), testGameConfig(), Options{
		CacheDir: t.TempDir(),
		ModDir:   modDir,
		Order:    conflict.LoadOrder{"mod_a", "mod_b"},
	}); err != nil {
		t.Fatalf("GeneratePatch: %v", err)
	}

	afterA, err := os.ReadFile(filepath.Join(modDir, "mod_a.mod"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(beforeA, afterA) {
		t.Error("mod_a's own descriptor was modified by GeneratePatch")
	}
}
