package main

import "runtime"

// Developer tools (Settings > Debug) are the web inspector for the interface, for debugging and
// for testing changes to it. They belong to development builds only: a release build is made
// without the inspector (no devtools build tag), the Debug tab is not offered in it, and the
// setting is ignored there even if a settings file asks for it.
//
// In a development build (wails dev, F5 in VS Code) the inspector is always there - its keyboard
// shortcut (F12, Ctrl+Shift+F12 on Linux) and the browser's own right-click menu, which the
// window allows in dev mode. What the setting adds is letting Shift+right-click through to that
// menu: the interface otherwise shows its own menus and never the browser's.

// DeveloperToolsStatus says what the Debug tab needs to know that the settings file does not.
type DeveloperToolsStatus struct {
	// Available: this is a development build, so the Debug tab exists.
	Available bool
	// Enabled is the developer tools setting; always false outside a development build.
	Enabled bool
	// OS is the operating system (runtime.GOOS), for the shortcut to show.
	OS string
}

// DeveloperToolsStatus reports whether the Debug tab is available and the state of its setting.
func (a *App) DeveloperToolsStatus() DeveloperToolsStatus {
	a.preferencesMu.Lock()
	enabled := a.preferences.DeveloperTools
	a.preferencesMu.Unlock()
	return DeveloperToolsStatus{
		Available: devBuild,
		Enabled:   devBuild && enabled,
		OS:        runtime.GOOS,
	}
}
