package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranslationCompanionModIDIsDeterministicAndSanitized(t *testing.T) {
	got := TranslationCompanionModID("some/mod/with/slashes")
	want := "parallax_translation_some_mod_with_slashes"
	if got != want {
		t.Errorf("TranslationCompanionModID() = %q, want %q", got, want)
	}
}

func TestTranslationCompanionModIDNeverUsesTheFiveZPatchPrefix(t *testing.T) {
	got := TranslationCompanionModID("my_mod")
	if strings.HasPrefix(got, "zzzzz") {
		t.Errorf("TranslationCompanionModID() = %q, must never use the patch mod's own aggressive-sort prefix", got)
	}
}

func TestResolveTranslationCompanionPaths(t *testing.T) {
	info := ResolveTranslationCompanion("/mods", "my_mod")
	if info.ModID != "parallax_translation_my_mod" {
		t.Errorf("ModID = %q", info.ModID)
	}
	if info.ContentDir != filepath.Join("/mods", "parallax_translation_my_mod") {
		t.Errorf("ContentDir = %q", info.ContentDir)
	}
	if info.StubPath != filepath.Join("/mods", "parallax_translation_my_mod.mod") {
		t.Errorf("StubPath = %q", info.StubPath)
	}
}

func TestEnsureTranslationCompanionWritesDescriptorStubAndManifest(t *testing.T) {
	modDir := t.TempDir()
	info := ResolveTranslationCompanion(modDir, "my_mod")

	if err := EnsureTranslationCompanion(info, "my_mod", "My Mod", "v4.4.6"); err != nil {
		t.Fatalf("EnsureTranslationCompanion() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(info.ContentDir, "descriptor.mod")); err != nil {
		t.Error("descriptor.mod was not written")
	}
	if _, err := os.Stat(info.StubPath); err != nil {
		t.Error("stub .mod was not written")
	}
	manifestPath := filepath.Join(info.ContentDir, translationCompanionManifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("manifest was not written: %v", err)
	}
	if !strings.Contains(string(data), `"sourceModId": "my_mod"`) {
		t.Errorf("manifest = %s, want it to name the source mod", data)
	}
	if !strings.Contains(string(data), `"generation": 1`) {
		t.Errorf("manifest = %s, want generation 1 on first run", data)
	}
}

func TestEnsureTranslationCompanionIncrementsGenerationOnASecondCall(t *testing.T) {
	modDir := t.TempDir()
	info := ResolveTranslationCompanion(modDir, "my_mod")

	if err := EnsureTranslationCompanion(info, "my_mod", "My Mod", ""); err != nil {
		t.Fatal(err)
	}
	if err := EnsureTranslationCompanion(info, "my_mod", "My Mod", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(info.ContentDir, translationCompanionManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"generation": 2`) {
		t.Errorf("manifest = %s, want generation 2 after a second call", data)
	}
}

func TestEnsureTranslationCompanionNeverWipesExistingLocaleContentFiles(t *testing.T) {
	// Unlike GeneratePatch, this must never delete the content directory
	// first - a real locale .yml file written by a previous translate run
	// must survive a later EnsureTranslationCompanion call for a different
	// language.
	modDir := t.TempDir()
	info := ResolveTranslationCompanion(modDir, "my_mod")
	if err := EnsureTranslationCompanion(info, "my_mod", "My Mod", ""); err != nil {
		t.Fatal(err)
	}

	localeFile := filepath.Join(info.ContentDir, "localisation", "german", "parallax_translation_my_mod_l_german.yml")
	if err := os.MkdirAll(filepath.Dir(localeFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localeFile, []byte("l_german:\n A:0 \"Hallo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureTranslationCompanion(info, "my_mod", "My Mod", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(localeFile); err != nil {
		t.Error("a previously-written locale file was removed by a later EnsureTranslationCompanion call")
	}
}

func TestEnsureTranslationCompanionDependsOnTheSourceModsDisplayName(t *testing.T) {
	modDir := t.TempDir()
	info := ResolveTranslationCompanion(modDir, "my_mod")
	if err := EnsureTranslationCompanion(info, "my_mod", "Real Space Battles", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(info.ContentDir, "descriptor.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Real Space Battles") {
		t.Errorf("descriptor.mod = %s, want it to name the source mod as a dependency", data)
	}
}
