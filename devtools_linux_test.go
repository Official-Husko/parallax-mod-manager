package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// The setting is read from the settings file before the window exists, and only counts in a
// build that includes the web inspector.
func TestDeveloperToolsSettingIsReadFromTheSettingsFile(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	dir := filepath.Join(config, "parallax-mod-manager")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "preferences.jsonc")

	if developerToolsSetting() {
		t.Error("no settings file: developer tools must be off")
	}
	p := preferences.Defaults()
	p.DeveloperTools = true
	if err := preferences.Save(path, p); err != nil {
		t.Fatal(err)
	}
	if got := developerToolsSetting(); got != devtoolsBuiltIn {
		t.Errorf("setting on: developerToolsSetting() = %v, want %v (whether this build has developer tools)", got, devtoolsBuiltIn)
	}
	if app := NewApp(); app.developerToolsAtStart != devtoolsBuiltIn {
		t.Errorf("NewApp remembered %v at start, want %v", app.developerToolsAtStart, devtoolsBuiltIn)
	}
}

func TestDeveloperToolsStatus(t *testing.T) {
	a := &App{preferences: preferences.Defaults()}
	st := a.DeveloperToolsStatus()
	if st.BuiltIn != devtoolsBuiltIn || st.Enabled || st.RestartNeeded || st.OS != "linux" {
		t.Errorf("fresh: %+v", st)
	}

	// Turned on since the window was created: the native menu is not set up for it yet.
	a.preferences.DeveloperTools = true
	st = a.DeveloperToolsStatus()
	if !st.Enabled || st.RestartNeeded != devtoolsBuiltIn {
		t.Errorf("turned on after start: %+v (a restart is needed only in a build that has developer tools)", st)
	}

	// On at start and still on: nothing to restart.
	a.developerToolsAtStart = true
	if st = a.DeveloperToolsStatus(); st.RestartNeeded {
		t.Errorf("on at start and still on: %+v", st)
	}
	// On at start, turned off since.
	a.preferences.DeveloperTools = false
	if st = a.DeveloperToolsStatus(); st.RestartNeeded != devtoolsBuiltIn {
		t.Errorf("turned off after start: %+v", st)
	}
}

func TestEnvironmentReportSaysWhetherDeveloperToolsAreOn(t *testing.T) {
	a := &App{preferences: preferences.Defaults()}
	joined := strings.Join(a.environmentReportLines(), "\n")
	want := "developer tools off"
	if !devtoolsBuiltIn {
		want += " (not in this build)"
	}
	if !strings.Contains(joined, want) {
		t.Errorf("the report lacks %q:\n%s", want, joined)
	}
}
