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

func TestStellarisSeedFieldsArePresent(t *testing.T) {
	if Stellaris.Key != "stellaris" {
		t.Errorf("Key = %q", Stellaris.Key)
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

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	g, ok := r.Get("stellaris")
	if !ok {
		t.Fatal("expected stellaris to be registered")
	}
	if !reflect.DeepEqual(g, Stellaris) {
		t.Errorf("Get(\"stellaris\") = %+v, want %+v", g, Stellaris)
	}

	if _, ok := r.Get("does-not-exist"); ok {
		t.Error("expected lookup of an unknown game to fail")
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
		Key:                "test-game",
		ExecutableFallback: ExecutableInfo{Path: "/opt/test-game/bin/test-game", Args: []string{"--fallback"}},
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
	g := GameConfig{Key: "test-game"}
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
		Key:                "test-game",
		ExecutableFallback: ExecutableInfo{Path: "/opt/test-game/bin/test-game"},
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
