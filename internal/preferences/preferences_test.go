package preferences

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	got := Load(path)
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load(missing) = %+v, want Defaults() = %+v", got, Defaults())
	}
}

func TestLoadCorruptFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.jsonc")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := Load(path)
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load(corrupt) = %+v, want Defaults() = %+v", got, Defaults())
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.jsonc")

	want := Preferences{
		ScanForNewMods:      false,
		CloseAfterLaunch:    true,
		WarnOnPatchMismatch: true,
		LastSelectedGame:    "01a0963a-c214-75a3-908d-1b76b91ea7bf",
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := Load(path)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-tripped = %+v, want %+v", got, want)
	}
}

func TestLoadToleratesJSONCComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.jsonc")
	content := `{
		// user turned this off after a big Workshop update
		"scanForNewMods": false,
		"closeAfterLaunch": true,
		"warnOnPatchMismatch": true,
		"lastSelectedGame": "",
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := Load(path)
	// What the file states wins; every setting it doesn't mention keeps its
	// default (see Load).
	want := Defaults()
	want.ScanForNewMods = false
	want.CloseAfterLaunch = true
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load(jsonc) = %+v, want %+v", got, want)
	}
}

func TestLoadKeepsDefaultsForSettingsAddedAfterTheFileWasSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	// An older file: it has never heard of autosortPatchLast.
	old := `{"scanForNewMods": false, "autosortDependencies": true}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if !got.AutosortPatchLast {
		t.Error("a setting the file doesn't mention must keep its default (on), not silently read as off")
	}
	if got.ScanForNewMods {
		t.Error("a setting the file does state must still win over the default")
	}
}

func TestLoadExplicitFalseOverridesAnOnByDefaultSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	if err := os.WriteFile(path, []byte(`{"autosortPatchLast": false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Load(path).AutosortPatchLast {
		t.Error("an explicit false must stay false")
	}
}

func TestDefaultsKeepTheGeneratedPatchLast(t *testing.T) {
	if !Defaults().AutosortPatchLast {
		t.Error("keeping the generated patch last must be on by default")
	}
}
