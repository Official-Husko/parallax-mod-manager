package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// Developer tools (Settings > Debug) are the web inspector for the interface, for
// debugging and for testing changes to it.
//
// What the framework allows: whether a build contains the inspector at all is decided when
// it is compiled (the devtools build tag, which build.sh passes, or a dev build), and the
// window's own keyboard shortcut for it (F12, Ctrl+Shift+F12 on Linux) cannot be turned off
// afterwards. What the setting controls is the rest: the browser's own right-click menu with
// Inspect Element (Shift+right-click), which the window is created with only when the
// setting is on - hence a restart to apply a change - and the button and hints in the Debug
// tab.

// developerToolsSetting reads the developer tools setting straight from the settings file,
// before the window exists: the option it feeds is fixed at creation, and the app's own
// startup runs after that.
func developerToolsSetting() bool {
	if !devtoolsBuiltIn {
		return false
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	return preferences.Load(filepath.Join(dir, "parallax-mod-manager", "preferences.jsonc")).DeveloperTools
}

// DeveloperToolsStatus says what the Debug tab needs to know that the settings file does not.
type DeveloperToolsStatus struct {
	// BuiltIn: this build includes the web inspector. Without it the setting does
	// nothing, and the tab says how to make a build that has it.
	BuiltIn bool
	// Enabled is the setting as it stands now.
	Enabled bool
	// RestartNeeded: the setting differs from what the window was created with.
	RestartNeeded bool
	// OS is the operating system (runtime.GOOS), for the shortcut to show.
	OS string
}

// DeveloperToolsStatus reports the state of the developer tools setting.
func (a *App) DeveloperToolsStatus() DeveloperToolsStatus {
	a.preferencesMu.Lock()
	enabled := a.preferences.DeveloperTools
	a.preferencesMu.Unlock()
	return DeveloperToolsStatus{
		BuiltIn:       devtoolsBuiltIn,
		Enabled:       enabled,
		RestartNeeded: devtoolsBuiltIn && enabled != a.developerToolsAtStart,
		OS:            runtime.GOOS,
	}
}
