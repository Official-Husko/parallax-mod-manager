package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.jsonc")
	got := Load(path)
	if got != Defaults() {
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
	if got != Defaults() {
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
	if got != want {
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
	want := Preferences{ScanForNewMods: false, CloseAfterLaunch: true, WarnOnPatchMismatch: true}
	if got != want {
		t.Errorf("Load(jsonc) = %+v, want %+v", got, want)
	}
}
