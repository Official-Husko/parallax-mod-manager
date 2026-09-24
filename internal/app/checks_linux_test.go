package app

import (
	"path/filepath"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/modcheck"
)

// TestCheckModReportsFilesReadAndTiming deliberately never lets resolveInstallDir fall through to
// real Steam-library detection: env.cfg is the real Stellaris GameConfig (a real Steam App ID),
// and on a machine that actually has Stellaris installed, cfg.DetectInstall() would find and
// scan it for real - exactly the kind of test-isolation leak this project has hit before (see
// the XDG_DATA_HOME lessons elsewhere). Setting an explicit, verifiable GamePaths override to a
// fake install directory makes resolveInstallDir use that instead, unconditionally, on every
// machine this test runs on.
func TestCheckModReportsFilesReadAndTiming(t *testing.T) {
	env := newEditorEnv(t)
	env.a.cacheDir = t.TempDir()

	install := t.TempDir()
	writeText(t, filepath.Join(install, "launcher-settings.json"), "{}") // env.cfg's own SignatureFiles entry
	writeText(t, filepath.Join(install, "common", "buildings", "00_buildings.txt"), `building_capital = { cost = 100 }`)
	env.a.preferences.GamePaths = map[string]string{env.cfg.ID: install}

	result, err := env.a.CheckMod(env.cfg.ID, "local_a", nil)
	if err != nil {
		t.Fatalf("CheckMod: %v", err)
	}
	if result.FilesRead == 0 {
		t.Error("FilesRead is 0, want at least local_a's own descriptor/common file")
	}
	if result.RanAt == 0 {
		t.Error("RanAt is 0, want a real unix timestamp")
	}
	if result.ResultBytes == 0 {
		t.Error("ResultBytes is 0, want a real marshalled-findings size")
	}
	if !result.BaseGameChecked {
		t.Error("BaseGameChecked is false, want true - a fake install dir was configured")
	}
	if result.BaseGameIndexFiles == 0 || result.BaseGameIndexBytes == 0 {
		t.Errorf("BaseGameIndex stats = %d files / %d bytes, want real positive numbers", result.BaseGameIndexFiles, result.BaseGameIndexBytes)
	}
}

// Note: there is deliberately no "no install found" counterpart to the test above.
// resolveInstallDir falls through to cfg.DetectInstall() - real Steam-library detection - the
// moment a GamePaths override fails verification, so on a machine that actually has Stellaris
// installed (this one included), even an invalid override still resolves to the real install.
// Forcing a reliably-empty result would need a seam resolveInstallDir does not have; out of
// scope here. checkBaseGameConflicts' own "InstallDir == \"\" returns no findings, no error" path
// is covered directly in internal/modcheck's own tests instead.

func TestRebuildBaseGameIndexClearsTheVanillaCacheEntry(t *testing.T) {
	env := newEditorEnv(t)
	env.a.cacheDir = t.TempDir()
	store := cache.FileStore{Dir: env.a.cacheDir}

	// Seed a fake "already indexed" base-game cache entry.
	seeded := &cache.ModCache{
		Version: cache.FormatVersion, ParserVersion: cache.ParserVersion,
		ModID: modcheck.VanillaModID, GameKey: env.cfg.ID,
		Files: map[string]cache.FileRecord{"a.txt": {Path: "a.txt"}},
	}
	if err := store.Save(env.a.baseContext(), seeded); err != nil {
		t.Fatalf("seeding the cache: %v", err)
	}
	if _, _, ok := store.Stat(env.cfg.ID, modcheck.VanillaModID); !ok {
		t.Fatal("the seeded entry was not found before rebuilding")
	}

	if err := env.a.RebuildBaseGameIndex(env.cfg.ID); err != nil {
		t.Fatalf("RebuildBaseGameIndex: %v", err)
	}
	if _, _, ok := store.Stat(env.cfg.ID, modcheck.VanillaModID); ok {
		t.Error("the vanilla cache entry still exists after RebuildBaseGameIndex")
	}

	// A game that does not exist is refused, not silently ignored.
	if err := env.a.RebuildBaseGameIndex("not-a-real-game-id"); err == nil {
		t.Error("RebuildBaseGameIndex for an unknown game succeeded, want an error")
	}
}
