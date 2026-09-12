// Package game holds per-game configuration: Steam app id, user data
// directory, descriptor format, and executable discovery. See
// docs/game-configuration.md.
package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/steam"
)

// DLCEntry is one piece of official DLC, listed so it can be toggled like a
// mod (see docs/game-launching.md).
type DLCEntry struct {
	ID   string
	Name string
}

// ExecutableInfo is how to launch a game directly (a fallback - the primary
// launch path is the Steam protocol URL, see docs/game-launching.md).
type ExecutableInfo struct {
	Path string
	Args []string
}

// GameConfig is everything the rest of the app needs to know about one
// supported Paradox game.
type GameConfig struct {
	Key         string // short identifier, e.g. "stellaris"
	DisplayName string
	SteamAppID  string
	// FolderName is the game's folder name under the Paradox user-data root,
	// e.g. "Stellaris" - see UserDataDir for how that root itself varies by OS.
	FolderName string

	DescriptorType mod.DescriptorType
	SignatureFiles []string // paths that must exist for a folder to count as a valid install
	ScanFolders    []string // top-level content folders that matter for mod scanning

	DLC []DLCEntry

	// LauncherSettingsPath is where this game's launcher-settings.json
	// lives, relative to its install directory. Most games keep it at the
	// install root ("launcher-settings.json"); some nest it under a
	// "launcher" subfolder instead - this must be set per game, not
	// assumed, since ResolveExecutable reads from exactly this path.
	LauncherSettingsPath string

	ExecutableFallback ExecutableInfo
}

// UserDataDir resolves this game's user data directory (holding the mod
// folder, saves, and - for classic-descriptor games - the load-order JSON
// files).
//
// Confirmed against a real Linux Stellaris install: its own
// launcher-settings.json reports `"gameDataPath": "$LINUX_DATA_HOME/Paradox
// Interactive/<Game>"`, and the actual directory exists at exactly the XDG
// data path this resolves to. On Linux this is a native path (Stellaris
// ships a native Linux binary - no Proton, no Steam compatdata prefix
// involved at all), respecting $XDG_DATA_HOME and defaulting to
// ~/.local/share. Windows keeps the standard Documents-folder convention.
// macOS uses the standard Application Support convention as a reasonable
// default, but that hasn't been verified against a real install.
func (g GameConfig) UserDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("game: resolving user home directory: %w", err)
	}

	switch runtime.GOOS {
	case "linux":
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(dataHome, "Paradox Interactive", g.FolderName), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Paradox Interactive", g.FolderName), nil
	default: // windows
		return filepath.Join(home, "Documents", "Paradox Interactive", g.FolderName), nil
	}
}

// launcherSettings mirrors the Paradox Launcher's own launcher-settings.json,
// which every install already has and which names the real executable -
// more resilient than hardcoding a binary name/path per OS. See
// docs/game-configuration.md.
type launcherSettings struct {
	ExePath      string   `json:"exePath"`
	ExeArgs      []string `json:"exeArgs"`
	GameDataPath string   `json:"gameDataPath"`
}

// ResolveExecutable reads installDir's launcher-settings.json for the real
// executable path/args; if that file is missing or unreadable, it falls back
// to g.ExecutableFallback.
func (g GameConfig) ResolveExecutable(installDir string) (ExecutableInfo, error) {
	data, err := os.ReadFile(filepath.Join(installDir, g.LauncherSettingsPath))
	if err != nil {
		if g.ExecutableFallback.Path == "" {
			return ExecutableInfo{}, fmt.Errorf("game: no %s in %s and no fallback executable configured for %s", g.LauncherSettingsPath, installDir, g.Key)
		}
		return g.ExecutableFallback, nil
	}

	var ls launcherSettings
	if err := json.Unmarshal(data, &ls); err != nil {
		if g.ExecutableFallback.Path == "" {
			return ExecutableInfo{}, fmt.Errorf("game: malformed launcher-settings.json in %s: %w", installDir, err)
		}
		return g.ExecutableFallback, nil
	}
	if ls.ExePath == "" {
		if g.ExecutableFallback.Path == "" {
			return ExecutableInfo{}, fmt.Errorf("game: launcher-settings.json in %s has no exePath and no fallback executable configured", installDir)
		}
		return g.ExecutableFallback, nil
	}

	return ExecutableInfo{Path: filepath.Join(installDir, ls.ExePath), Args: ls.ExeArgs}, nil
}

// DetectInstall searches this machine's default Steam libraries for g's
// install folder and confirms every one of g.SignatureFiles actually exists
// there before reporting it found - stale Steam library bookkeeping (an
// uninstalled game, a moved drive) must not produce a false positive.
// Returns ("", false), never a guess, when nothing checks out.
func (g GameConfig) DetectInstall() (string, bool) {
	dir, err := steam.FindGameInstallDir(g.SteamAppID)
	if err != nil {
		return "", false
	}
	if !g.VerifyInstallDir(dir) {
		return "", false
	}
	return dir, true
}

// VerifyInstallDir reports whether every one of g.SignatureFiles exists
// under dir - the same check DetectInstall uses, exposed on its own so a
// manually browsed-to folder (auto-detection didn't find it, or found the
// wrong thing) can be confirmed before it's trusted as a real install.
func (g GameConfig) VerifyInstallDir(dir string) bool {
	for _, sig := range g.SignatureFiles {
		if _, err := os.Stat(filepath.Join(dir, sig)); err != nil {
			return false
		}
	}
	return true
}
