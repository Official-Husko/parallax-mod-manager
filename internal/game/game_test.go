package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func TestStellarisFixtureFieldsArePresent(t *testing.T) {
	if Stellaris.ID == "" {
		t.Error("ID is empty")
	}
	if Stellaris.SteamAppID == "" {
		t.Error("SteamAppID is empty")
	}
	if Stellaris.DescriptorType != mod.DescriptorClassic {
		t.Errorf("DescriptorType = %v, want DescriptorClassic", Stellaris.DescriptorType)
	}
	if len(Stellaris.ScanFolders) == 0 {
		t.Error("ScanFolders is empty")
	}
	if len(Stellaris.SignatureFiles) == 0 {
		t.Error("SignatureFiles is empty")
	}
}

// TestRegistryGetAndList exercises Registry as a plain container - the
// real games list's own validity is covered by gamelist_test.go's
// TestRealGamesListFileIsValidAndMatchesTheFixture and the parseGameList/
// LoadRegistry tests there.
func TestRegistryGetAndList(t *testing.T) {
	r := NewRegistry([]GameConfig{Stellaris})
	g, ok := r.Get(Stellaris.ID)
	if !ok {
		t.Fatal("expected Stellaris to be registered")
	}
	if !reflect.DeepEqual(g, Stellaris) {
		t.Errorf("Get(%q) = %+v, want %+v", Stellaris.ID, g, Stellaris)
	}

	if _, ok := r.Get("does-not-exist"); ok {
		t.Error("expected lookup of an unknown game to fail")
	}

	list := r.List()
	if len(list) != 1 || list[0].ID != Stellaris.ID {
		t.Errorf("List() = %+v", list)
	}
}

func TestResolveExecutableReadsLauncherSettings(t *testing.T) {
	installDir := t.TempDir()
	settings := map[string]any{
		"exePath": "stellaris",
		"exeArgs": []string{"-skiplauncher"},
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "launcher-settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := Stellaris.ResolveExecutable(installDir)
	if err != nil {
		t.Fatalf("ResolveExecutable: %v", err)
	}
	want := ExecutableInfo{Path: filepath.Join(installDir, "stellaris"), Args: []string{"-skiplauncher"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveExecutable = %+v, want %+v", got, want)
	}
}

func TestResolveExecutableFallsBackWhenLauncherSettingsMissing(t *testing.T) {
	g := GameConfig{
		ID:                   "test-game",
		LauncherSettingsPath: "launcher-settings.json",
		ExecutableFallback:   ExecutableInfo{Path: "/opt/test-game/bin/test-game", Args: []string{"--fallback"}},
	}
	installDir := t.TempDir() // no launcher-settings.json present

	got, err := g.ResolveExecutable(installDir)
	if err != nil {
		t.Fatalf("ResolveExecutable: %v", err)
	}
	if !reflect.DeepEqual(got, g.ExecutableFallback) {
		t.Errorf("ResolveExecutable = %+v, want fallback %+v", got, g.ExecutableFallback)
	}
}

func TestResolveExecutableErrorsWithNoSettingsAndNoFallback(t *testing.T) {
	g := GameConfig{ID: "test-game", LauncherSettingsPath: "launcher-settings.json"}
	_, err := g.ResolveExecutable(t.TempDir())
	if err == nil {
		t.Fatal("expected an error when there's no launcher-settings.json and no fallback")
	}
}

func TestResolveExecutableFallsBackOnMalformedSettings(t *testing.T) {
	installDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(installDir, "launcher-settings.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	g := GameConfig{
		ID:                   "test-game",
		LauncherSettingsPath: "launcher-settings.json",
		ExecutableFallback:   ExecutableInfo{Path: "/opt/test-game/bin/test-game"},
	}
	got, err := g.ResolveExecutable(installDir)
	if err != nil {
		t.Fatalf("ResolveExecutable: %v", err)
	}
	if !reflect.DeepEqual(got, g.ExecutableFallback) {
		t.Errorf("ResolveExecutable = %+v, want fallback", got)
	}
}

func TestUserDataDirIncludesFolderName(t *testing.T) {
	dir, err := Stellaris.UserDataDir()
	if err != nil {
		t.Fatalf("UserDataDir: %v", err)
	}
	if filepath.Base(dir) != "Stellaris" {
		t.Errorf("UserDataDir = %q, want it to end in .../Stellaris", dir)
	}
}

func TestUserDataDirLinuxUsesXDGDataHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific path resolution")
	}
	t.Setenv("XDG_DATA_HOME", "/custom/data/home")

	dir, err := Stellaris.UserDataDir()
	if err != nil {
		t.Fatalf("UserDataDir: %v", err)
	}
	want := filepath.Join("/custom/data/home", "Paradox Interactive", "Stellaris")
	if dir != want {
		t.Errorf("UserDataDir = %q, want %q", dir, want)
	}
}

func TestUserDataDirLinuxDefaultsToLocalShare(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific path resolution")
	}
	t.Setenv("XDG_DATA_HOME", "")

	dir, err := Stellaris.UserDataDir()
	if err != nil {
		t.Fatalf("UserDataDir: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	// Confirmed against a real install: ~/.local/share/Paradox
	// Interactive/Stellaris is exactly where Stellaris's own
	// launcher-settings.json ("$LINUX_DATA_HOME/Paradox
	// Interactive/Stellaris") resolves to when $XDG_DATA_HOME is unset.
	want := filepath.Join(home, ".local", "share", "Paradox Interactive", "Stellaris")
	if dir != want {
		t.Errorf("UserDataDir = %q, want %q", dir, want)
	}
}

func TestUserDataDirDarwinPrefersDocumentsWhenItReallyExists(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-specific path resolution")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	documents := filepath.Join(home, "Documents", "Paradox Interactive", "Stellaris")
	if err := os.MkdirAll(documents, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Join(home, "Documents", "Paradox Interactive")) })

	dir, err := Stellaris.UserDataDir()
	if err != nil {
		t.Fatalf("UserDataDir: %v", err)
	}
	if dir != documents {
		t.Errorf("UserDataDir = %q, want the real, existing %q", dir, documents)
	}
}

func TestUserDataDirDarwinFallsBackToApplicationSupportWhenDocumentsFormMissing(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-specific path resolution")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	// Guard against a real Documents/Paradox Interactive/Stellaris folder
	// actually existing on whatever machine runs this test, which would
	// make the "missing" premise false and the assertion below wrong for
	// reasons unrelated to the code under test.
	if _, err := os.Stat(filepath.Join(home, "Documents", "Paradox Interactive", "Stellaris")); err == nil {
		t.Skip("a real Documents/Paradox Interactive/Stellaris exists on this machine - fallback case not exercisable here")
	}

	dir, err := Stellaris.UserDataDir()
	if err != nil {
		t.Fatalf("UserDataDir: %v", err)
	}
	want := filepath.Join(home, "Library", "Application Support", "Paradox Interactive", "Stellaris")
	if dir != want {
		t.Errorf("UserDataDir = %q, want %q", dir, want)
	}
}

func TestGameVersionReadsRawVersion(t *testing.T) {
	installDir := t.TempDir()
	// Confirmed real shape (a live Stellaris install's own
	// launcher-settings.json): "version" is a human-readable display
	// string with a codename and build hash, "rawVersion" is the precise,
	// wildcard-comparable one this project actually uses.
	settings := map[string]any{
		"version":                  "Pegasus v4.4.6 (fdde)",
		"rawVersion":               "v4.4.6",
		"modsCompatibilityVersion": "4.4",
	}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "launcher-settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := Stellaris.GameVersion(installDir)
	if got != "v4.4.6" {
		t.Errorf("GameVersion = %q, want %q", got, "v4.4.6")
	}
}

func TestGameVersionEmptyWhenSettingsMissing(t *testing.T) {
	got := Stellaris.GameVersion(t.TempDir())
	if got != "" {
		t.Errorf("GameVersion = %q, want empty (no launcher-settings.json at all)", got)
	}
}

func TestGameVersionEmptyWhenSettingsMalformed(t *testing.T) {
	installDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(installDir, "launcher-settings.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := Stellaris.GameVersion(installDir)
	if got != "" {
		t.Errorf("GameVersion = %q, want empty (malformed settings)", got)
	}
}

func TestGameVersionEmptyWhenFieldMissing(t *testing.T) {
	installDir := t.TempDir()
	// A real settings file that simply predates rawVersion, or belongs to
	// a game/launcher version that never wrote one.
	data, err := json.Marshal(map[string]any{"exePath": "stellaris"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "launcher-settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := Stellaris.GameVersion(installDir)
	if got != "" {
		t.Errorf("GameVersion = %q, want empty (no rawVersion field present)", got)
	}
}

// writeAppManifest writes a real-format Steam app manifest (VDF despite the
// .acf extension) into steamappsDir, confirmed against an actual
// appmanifest_281990.acf on a real Stellaris install.
func writeAppManifest(t *testing.T, steamappsDir, appID, installDirName string) {
	t.Helper()
	if err := os.MkdirAll(steamappsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	acf := `"AppState"
{
	"appid"		"` + appID + `"
	"name"		"Test Game"
	"installdir"		"` + installDirName + `"
}
`
	path := filepath.Join(steamappsDir, "appmanifest_"+appID+".acf")
	if err := os.WriteFile(path, []byte(acf), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestDetectInstallFindsRealInstallWithSignatureFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	steamapps := filepath.Join(home, ".steam", "steam", "steamapps")
	writeAppManifest(t, steamapps, Stellaris.SteamAppID, "Stellaris")
	installDir := filepath.Join(steamapps, "common", "Stellaris")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "launcher-settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dir, found := Stellaris.DetectInstall()
	if !found {
		t.Fatal("expected DetectInstall to find the install")
	}
	if dir != installDir {
		t.Errorf("DetectInstall dir = %q, want %q", dir, installDir)
	}
}

func TestDetectInstallFalseWhenSignatureFileMissing(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	steamapps := filepath.Join(home, ".steam", "steam", "steamapps")
	writeAppManifest(t, steamapps, Stellaris.SteamAppID, "Stellaris")
	installDir := filepath.Join(steamapps, "common", "Stellaris")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// No launcher-settings.json written - the folder exists (e.g. a stale
	// or partial install) but the required signature file doesn't.

	if _, found := Stellaris.DetectInstall(); found {
		t.Error("expected DetectInstall to report not found without the signature file")
	}
}

func TestVerifyInstallDir(t *testing.T) {
	dir := t.TempDir()
	if Stellaris.VerifyInstallDir(dir) {
		t.Error("expected VerifyInstallDir to be false without launcher-settings.json")
	}
	if err := os.WriteFile(filepath.Join(dir, "launcher-settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !Stellaris.VerifyInstallDir(dir) {
		t.Error("expected VerifyInstallDir to be true once the signature file exists")
	}
}

func TestDetectInstallFalseWhenNoLibraryHasIt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	t.Setenv("HOME", t.TempDir())

	if _, found := Stellaris.DetectInstall(); found {
		t.Error("expected DetectInstall to report not found with no Steam library at all")
	}
}
