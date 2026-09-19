// This file's tests never construct OSLauncher{} and never call
// GameConfig.UserDataDir() - every scenario here uses *FakeLauncher and a
// t.TempDir()-backed launcher-settings.json fixture instead, so nothing in
// this package's test suite can ever actually invoke Steam, spawn a
// process, or touch a real installed game.
package launch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
)

func TestLaunchUsesSteamURLWhenAppIDSet(t *testing.T) {
	fake := &FakeLauncher{}
	cfg := testClassicGame()

	if err := Launch(fake, cfg, LaunchOptions{ExtraArgs: []string{"-skiplauncher"}}); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	want := []string{SteamAppURL(cfg, []string{"-skiplauncher"})}
	if !reflect.DeepEqual(fake.OpenedURLs, want) {
		t.Errorf("OpenedURLs = %v, want %v", fake.OpenedURLs, want)
	}
	if len(fake.RanExecutables) != 0 {
		t.Errorf("RanExecutables = %+v, want none (Steam path should not touch the direct-exe fallback)", fake.RanExecutables)
	}
}

func writeLauncherSettings(t *testing.T, dir string) {
	t.Helper()
	settings := map[string]any{"exePath": "stellaris", "exeArgs": []string{"-base-arg"}}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "launcher-settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLaunchFallsBackToDirectExeWhenNoSteamAppID(t *testing.T) {
	installDir := t.TempDir()
	writeLauncherSettings(t, installDir)

	fake := &FakeLauncher{}
	cfg := testClassicGame()
	cfg.SteamAppID = ""

	if err := Launch(fake, cfg, LaunchOptions{InstallDir: installDir}); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if len(fake.OpenedURLs) != 0 {
		t.Errorf("OpenedURLs = %v, want none", fake.OpenedURLs)
	}
	want := game.ExecutableInfo{Path: filepath.Join(installDir, "stellaris"), Args: []string{"-base-arg"}, WorkingDir: installDir}
	if len(fake.RanExecutables) != 1 || !reflect.DeepEqual(fake.RanExecutables[0], want) {
		t.Errorf("RanExecutables = %+v, want [%+v]", fake.RanExecutables, want)
	}
}

func TestLaunchDirectOptionForcesFallbackEvenWithSteamAppID(t *testing.T) {
	installDir := t.TempDir()
	writeLauncherSettings(t, installDir)

	fake := &FakeLauncher{}
	cfg := testClassicGame() // has a SteamAppID

	if err := Launch(fake, cfg, LaunchOptions{Direct: true, InstallDir: installDir}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if len(fake.OpenedURLs) != 0 {
		t.Errorf("OpenedURLs = %v, want none (Direct should force the exe fallback)", fake.OpenedURLs)
	}
	if len(fake.RanExecutables) != 1 {
		t.Errorf("RanExecutables = %+v, want exactly one entry", fake.RanExecutables)
	}
}

func TestLaunchDirectExtraArgsAppended(t *testing.T) {
	installDir := t.TempDir()
	writeLauncherSettings(t, installDir)

	fake := &FakeLauncher{}
	cfg := testClassicGame()
	cfg.SteamAppID = ""

	if err := Launch(fake, cfg, LaunchOptions{InstallDir: installDir, ExtraArgs: []string{"-extra"}}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	want := []string{"-base-arg", "-extra"}
	if len(fake.RanExecutables) != 1 || !reflect.DeepEqual(fake.RanExecutables[0].Args, want) {
		t.Errorf("Args = %v, want %v", fake.RanExecutables[0].Args, want)
	}
}

func TestLaunchPropagatesLauncherError(t *testing.T) {
	fake := &FakeLauncher{OpenURLErr: errors.New("boom")}
	err := Launch(fake, testClassicGame(), LaunchOptions{})
	if err == nil {
		t.Fatal("expected Launch to propagate the Launcher's error")
	}
}

func TestLaunchPropagatesResolveExecutableError(t *testing.T) {
	fake := &FakeLauncher{}
	cfg := testClassicGame()
	cfg.SteamAppID = ""
	// No launcher-settings.json and no fallback configured.
	err := Launch(fake, cfg, LaunchOptions{InstallDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected Launch to propagate ResolveExecutable's error")
	}
	if len(fake.RanExecutables) != 0 {
		t.Errorf("RanExecutables = %+v, want none when resolution fails", fake.RanExecutables)
	}
}
