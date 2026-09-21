package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/about"
	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/backup"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
	"github.com/Official-Husko/parallax-mod-manager/internal/sysinfo"
)

// The start of every run's activity log carries a report of what the rest of the log is
// about: the build, the computer (operating system, session, webview, processor, memory,
// graphics) and the app's own surroundings (folders and their free space, Steam, the
// settings that change behaviour, the games found). It is written under the component
// "System" and pinned (applog.Logger.Pin), so it stays at the top of the log view however
// long the run gets, and it is in app.log at the start of every session.
//
// It goes to the local log only. Nothing in it identifies the person (no computer name,
// user name, serial number, address or machine id, and no Steam key or account id), and
// when sharing a log exists (docs/log-sharing.md) the lines of this component are what
// the "include computer details" option governs.

// systemLog is the logger the report is written with.
func systemLog() applog.Logger { return applog.For("System").Pin() }

// logSystemReport writes the build and computer part of the report: cheap, read from files
// the operating system keeps, so it runs at once and comes first.
func (a *App) logSystemReport(build about.Info) {
	log := systemLog()
	for _, line := range systemReportLines(build, sysinfo.Collect()) {
		log.Infof("%s", line)
	}
}

// systemReportLines are the report's build and computer lines.
func systemReportLines(build about.Info, sys sysinfo.Info) []string {
	parts := []string{fmt.Sprintf("%s %s", build.Name, build.Version)}
	if build.Commit != "" {
		c := "commit " + build.Commit
		if build.Dirty {
			c += " (uncommitted changes)"
		}
		parts = append(parts, c)
	}
	if build.GoVersion != "" {
		parts = append(parts, "Go "+build.GoVersion)
	}
	if build.WailsVersion != "" {
		parts = append(parts, "Wails "+build.WailsVersion)
	}
	if build.OS != "" {
		parts = append(parts, build.OS+"/"+build.Arch)
	}
	lines := []string{"Build: " + strings.Join(parts, ", ")}
	return append(lines, sys.Lines()...)
}

// logEnvironmentReport writes the part of the report that needs the app's state: folders,
// Steam, settings and the games found. It reads a few folders, so it runs off the startup
// path; its lines still land within milliseconds of the first ones.
func (a *App) logEnvironmentReport() {
	log := systemLog()
	for _, line := range a.environmentReportLines() {
		log.Infof("%s", line)
	}
}

// pathWithFree is "'path' (12.3 GiB free)", or just the quoted path when the space
// cannot be found out.
func pathWithFree(path string) string {
	if path == "" {
		return ""
	}
	if free, ok := backup.FreeSpace(path); ok {
		return fmt.Sprintf("'%s' (%s free)", path, sysinfo.HumanBytes(int64(free)))
	}
	return fmt.Sprintf("'%s'", path)
}

func onOffText(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// environmentReportLines are the report's lines about the app's surroundings and settings.
func (a *App) environmentReportLines() []string {
	var lines []string

	var folders []string
	if a.configAppDir != "" {
		folders = append(folders, "settings "+pathWithFree(a.configAppDir))
	}
	if a.cacheDir != "" {
		folders = append(folders, "cache "+pathWithFree(a.cacheDir))
	}
	if a.logDir != "" {
		folders = append(folders, fmt.Sprintf("log '%s'", a.logDir))
	}
	if len(folders) > 0 {
		lines = append(lines, "Folders: "+strings.Join(folders, ", "))
	}

	if len(a.steamRoots) == 0 {
		lines = append(lines, "Steam: no installation found")
	} else {
		quoted := make([]string, len(a.steamRoots))
		for i, r := range a.steamRoots {
			quoted[i] = "'" + r + "'"
		}
		lines = append(lines, fmt.Sprintf("Steam: %d %s: %s", len(a.steamRoots), pluralWord(len(a.steamRoots), "installation", "installations"), strings.Join(quoted, ", ")))
	}

	a.steam.mu.Lock()
	steamMode, hasKey := a.steam.settings.Mode, a.steam.settings.SealedKey != ""
	a.steam.mu.Unlock()
	switch {
	case steamMode == steamapi.ModeFree || steamMode == "":
		lines = append(lines, "Steam API: free API only")
	case hasKey:
		lines = append(lines, fmt.Sprintf("Steam API: %s use, a key is saved", steamMode))
	default:
		lines = append(lines, fmt.Sprintf("Steam API: %s use", steamMode))
	}

	a.backup.mu.Lock()
	bs, root := a.backup.settings, a.backup.rootLocked()
	a.backup.mu.Unlock()
	limit := "size limit off"
	if bs.LimitEnabled {
		limit = "size limit " + sysinfo.HumanBytes(bs.LimitBytes)
	}
	guard := "free-space guard off"
	if bs.KeepFreeEnabled {
		guard = "keeps " + sysinfo.HumanBytes(bs.KeepFreeBytes) + " free"
	}
	if root != "" {
		lines = append(lines, fmt.Sprintf("Backups: %s, folder %s, %s, %s", bs.Mode, pathWithFree(root), limit, guard))
	} else {
		lines = append(lines, fmt.Sprintf("Backups: %s, no folder", bs.Mode))
	}

	a.preferencesMu.Lock()
	p := clonePreferences(a.preferences)
	a.preferencesMu.Unlock()
	background := "off"
	if !p.BackgroundDisabled {
		background = p.BackgroundSource
		if background == "" {
			background = "online"
		}
		if p.BackgroundRotationPaused {
			background += ", not rotating"
		} else if p.BackgroundIntervalSeconds > 0 {
			background += fmt.Sprintf(", every %ds", p.BackgroundIntervalSeconds)
		}
	}
	devtools := onOffText(p.DeveloperTools)
	if !devtoolsBuiltIn {
		devtools += " (not in this build)"
	}
	lines = append(lines, fmt.Sprintf("Settings: watch for new mods %s, close after launch %s, background %s, autosort dependencies %s / fixes last %s / patch last %s, developer tools %s",
		onOffText(p.ScanForNewMods), onOffText(p.CloseAfterLaunch), background,
		onOffText(p.AutosortDependencies), onOffText(p.AutosortFixesLast), onOffText(p.AutosortPatchLast), devtools))

	if a.registry == nil {
		return lines
	}
	games := a.registry.List()
	managed := map[string]bool{}
	for _, id := range p.ManagedGames {
		managed[id] = true
	}
	installed, managedCount := 0, 0
	var gameLines []string
	for _, cfg := range games {
		isManaged := len(p.ManagedGames) == 0 || managed[cfg.ID]
		if isManaged {
			managedCount++
		}
		dir, ok := a.resolveInstallDir(cfg)
		if !ok {
			continue
		}
		installed++
		how := "found automatically"
		if _, overridden := a.gamePathOverride(cfg); overridden {
			how = "path chosen by hand"
		}
		version := cfg.GameVersion(dir)
		if version == "" {
			version = "version unknown"
		}
		modDir := "mod folder unknown"
		if data, err := cfg.UserDataDir(); err == nil {
			state := "missing"
			if info, err := os.Stat(filepath.Join(data, "mod")); err == nil && info.IsDir() {
				state = "present"
			}
			modDir = fmt.Sprintf("mod folder '%s' (%s)", filepath.Join(data, "mod"), state)
		}
		launch := p.LaunchModes[cfg.ID]
		if launch == "" {
			launch = "steam"
		}
		gameLines = append(gameLines, fmt.Sprintf("Game '%s': %s, installed at '%s' (%s), launch mode %s, %s%s",
			cfg.DisplayName, version, dir, how, launch, modDir, managedNote(isManaged)))
	}
	lines = append(lines, fmt.Sprintf("Games: %d registered, %d managed, %d installed", len(games), managedCount, installed))
	return append(lines, gameLines...)
}

func managedNote(managed bool) string {
	if managed {
		return ""
	}
	return ", not managed"
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
