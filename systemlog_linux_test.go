package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/about"
	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
	"github.com/Official-Husko/parallax-mod-manager/internal/sysinfo"
)

func TestSystemReportLinesStartWithTheBuildThenTheMachine(t *testing.T) {
	lines := systemReportLines(
		about.Info{Name: "Parallax Mod Manager", Version: "1.0.0", Commit: "29ee28e", Dirty: true, GoVersion: "1.27.0", WailsVersion: "2.15.0", OS: "linux", Arch: "amd64"},
		sysinfo.Info{OS: "EndeavourOS", Kernel: "Linux 6.18", CPU: "Test CPU", Threads: 16, MemoryTotal: 32 << 30, GPUs: []sysinfo.GPU{{Name: "Test GPU", Driver: "amdgpu"}}},
	)
	want := []string{
		"Build: Parallax Mod Manager 1.0.0, commit 29ee28e (uncommitted changes), Go 1.27.0, Wails 2.15.0, linux/amd64",
		"OS: EndeavourOS (Linux 6.18)",
		"CPU: Test CPU, 16 threads",
		"Memory: 32.0 GiB total",
		"GPU: Test GPU, driver amdgpu",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines =\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if got := systemReportLines(about.Info{Name: "X", Version: "1"}, sysinfo.Info{})[0]; got != "Build: X 1" {
		t.Errorf("a build with no details = %q", got)
	}
}

func TestEnvironmentReportSaysWhatMattersForDebugging(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("HOME", t.TempDir()) // no real Steam
	install := t.TempDir()
	if err := os.MkdirAll(filepath.Join(install, "launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(install, "launcher", "launcher-settings.json"), []byte(`{"rawVersion":"v4.4.6"}`), 0o644)
	modDir := filepath.Join(dataHome, "Paradox Interactive", "TestGame", "mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := game.GameConfig{ID: "test-game", DisplayName: "Test Game", FolderName: "TestGame", DescriptorType: mod.DescriptorClassic, LauncherSettingsPath: "launcher/launcher-settings.json"}
	other := game.GameConfig{ID: "other", DisplayName: "Other Game", FolderName: "Other"}

	prefs := preferences.Defaults()
	prefs.ManagedGames = []string{"test-game"}
	prefs.GamePaths = map[string]string{}
	a := &App{ctx: context.Background(), registry: game.NewRegistry([]game.GameConfig{cfg, other}), preferences: prefs, steamRoots: []string{"/games/Steam"}}
	config := t.TempDir()
	a.configAppDir, a.cacheDir, a.logDir = config, t.TempDir(), filepath.Join(t.TempDir(), "logs")
	a.initBackups(config)
	t.Cleanup(a.stopBackups)
	a.steam.settings.Mode = steamapi.ModeBackup
	a.steam.settings.SealedKey = "v1.secret-ciphertext.abc"
	a.steam.settings.Fingerprint = "deadbeef"

	// Point the install at the fake game through a manual path.
	a.preferences.GamePaths = map[string]string{"test-game": install}
	joined := strings.Join(a.environmentReportLines(), "\n")

	for _, want := range []string{
		"Folders: settings '" + config + "'",
		"Steam: 1 installation: '/games/Steam'",
		"Steam API: backup use, a key is saved",
		"Backups: atrisk, folder '" + filepath.Join(config, "Parallax Mod Backups") + "'",
		"size limit off, keeps 1.0 GiB free",
		"Settings: watch for new mods",
		"Games: 2 registered, 1 managed, 1 installed",
		"Game 'Test Game': v4.4.6, installed at '" + install + "' (path chosen by hand), launch mode steam, mod folder '" + modDir + "' (present)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the report lacks %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Other Game") {
		t.Error("a game that is not installed got a line of its own")
	}
	// Nothing secret.
	for _, secret := range []string{"secret-ciphertext", "deadbeef", "v1."} {
		if strings.Contains(joined, secret) {
			t.Errorf("the report contains %q", secret)
		}
	}
}

func TestEnvironmentReportWithNothingConfiguredStillWorks(t *testing.T) {
	a := &App{}
	lines := a.environmentReportLines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Steam: no installation found") || !strings.Contains(joined, "Steam API: free API only") {
		t.Errorf("lines = %s", joined)
	}
}

func TestTheReportIsPinnedToTheLogAndSurvivesItFillingUp(t *testing.T) {
	applog.Default().Clear()
	a := &App{ctx: context.Background()}
	a.logSystemReport(about.Info{Name: "Parallax Mod Manager", Version: "1.0.0"})
	for i := 0; i < applog.DefaultCapacity+50; i++ {
		applog.For("Scan").Infof("noise %d", i)
	}
	entries := applog.Default().Entries()
	if len(entries) == 0 || entries[0].Component != "System" || !strings.HasPrefix(entries[0].Message, "Build: Parallax Mod Manager 1.0.0") {
		t.Fatalf("the first entry is %+v, want the build line of the report", entries[0])
	}
	systemLines := 0
	for _, e := range entries {
		if e.Component == "System" {
			systemLines++
		}
	}
	if systemLines < 4 {
		t.Errorf("%d System lines survive, want the whole report (build, OS, CPU, memory ...)", systemLines)
	}
	applog.Default().Clear()
}
