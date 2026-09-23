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
	// WorkingDir is the directory the process should start in - always the
	// executable's own directory, resolved by ResolveExecutable itself,
	// never left for the OS default (the current process's own directory)
	// to decide. Without this, a game whose own code resolves paths
	// relative to its working directory (loading a library, finding its
	// own data files) would instead resolve them against wherever
	// Parallax Mod Manager's own process happens to be running from.
	WorkingDir string
}

// GameConfig is everything the rest of the app needs to know about one
// supported Paradox game.
type GameConfig struct {
	ID          string // permanent UUID identity - see docs/game-configuration.md
	DisplayName string
	SteamAppID  string
	// FolderName is the game's folder name under the Paradox user-data root,
	// e.g. "Stellaris" - see UserDataDir for how that root itself varies by OS.
	FolderName string

	DescriptorType mod.DescriptorType
	SignatureFiles []string // paths that must exist for a folder to count as a valid install
	ScanFolders    []string // top-level content folders that matter for mod scanning

	DLC []DLCEntry

	// ChecksumAlgorithm names how this game's multiplayer checksum is calculated (see
	// internal/checksum: "stellaris", "hoi4"); "" for a game whose scheme is not known, which
	// simply gets no checksum.
	ChecksumAlgorithm string

	// LauncherSettingsPath is where this game's launcher-settings.json
	// lives, relative to its install directory. Most games keep it at the
	// install root ("launcher-settings.json"); some nest it under a
	// "launcher" subfolder instead - this must be set per game, not
	// assumed, since ResolveExecutable reads from exactly this path.
	LauncherSettingsPath string

	// LauncherShimSupported gates the "Steam Direct" launch mode (see
	// internal/launchershim and companions/launcher-shim) - installing a shim in
	// place of this game's own dowser/dowser.exe. False (never touch this game's
	// launcher entry point) until actually verified against a real install: dowser
	// and launcher-settings.json are confirmed colocated for Stellaris and Hearts
	// of Iron IV, but not yet checked for CK3/Imperator: Rome/Victoria 3, which nest
	// launcher-settings.json under a "launcher" subfolder - it isn't yet confirmed
	// whether dowser sits there too, or at the install root instead.
	LauncherShimSupported bool

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
//
// macOS is still unverified against a real install either way - see
// docs/game-configuration.md's "macOS specifics" section. Two real,
// plausible conventions exist (Documents, matching Windows; Application
// Support, the more typical macOS convention for this kind of state), so
// rather than committing to one unconfirmed guess, this checks whether the
// Documents form actually exists first and only falls back to Application
// Support when it doesn't - correct the moment either guess is confirmed,
// without needing a code change to pick the winner.
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
		documents := filepath.Join(home, "Documents", "Paradox Interactive", g.FolderName)
		if _, err := os.Stat(documents); err == nil {
			return documents, nil
		}
		return filepath.Join(home, "Library", "Application Support", "Paradox Interactive", g.FolderName), nil
	default: // windows
		return filepath.Join(home, "Documents", "Paradox Interactive", g.FolderName), nil
	}
}

// launcherSettings mirrors the Paradox Launcher's own launcher-settings.json,
// which every install already has and which names the real executable -
// more resilient than hardcoding a binary name/path per OS. See
// docs/game-configuration.md.
//
// RawVersion is confirmed against a real Stellaris install: a precise,
// wildcard-comparable string like "v4.4.6" - the same "vX.Y.Z" shape a
// mod's own supported_version field uses (see
// frontend/src/data/versionCompat.ts, which compares the two client-side),
// confirmed present both with and without the leading "v" in real
// installed mods. modsCompatibilityVersion (a coarser "4.4") also exists in
// the same file but isn't used here - RawVersion alone already has every
// segment a supported_version pattern could pin against.
type launcherSettings struct {
	ExePath      string   `json:"exePath"`
	ExeArgs      []string `json:"exeArgs"`
	GameDataPath string   `json:"gameDataPath"`
	RawVersion   string   `json:"rawVersion"`
}

// GameVersion reads installDir's launcher-settings.json for the real,
// currently-installed game version, or "" if it's missing, unreadable, or
// simply doesn't carry a rawVersion - never an error: not every install
// has run through a launcher that wrote one, and "no version detected" is
// a normal, expected outcome for this project to just quietly not show
// anything for, not a failure to report.
func (g GameConfig) GameVersion(installDir string) string {
	data, err := os.ReadFile(filepath.Join(installDir, g.LauncherSettingsPath))
	if err != nil {
		return ""
	}
	var ls launcherSettings
	if err := json.Unmarshal(data, &ls); err != nil {
		return ""
	}
	return ls.RawVersion
}

// ResolveExecutable reads installDir's launcher-settings.json for the real
// executable path/args; if that file is missing or unreadable, it falls back
// to g.ExecutableFallback.
//
// exePath is resolved relative to launcher-settings.json's own directory,
// not installDir itself - for most games (Stellaris, EU4, HOI4) the two are
// the same, since launcher-settings.json sits at the install root, but CK3,
// Imperator: Rome, and Victoria 3 nest it under a "launcher/" subfolder and
// give exePath as a "../binaries/<game>.exe" style path relative to that
// subfolder - joining it against installDir directly (the previous
// behavior) pointed outside the install entirely for those three.
func (g GameConfig) ResolveExecutable(installDir string) (ExecutableInfo, error) {
	settingsPath := filepath.Join(installDir, g.LauncherSettingsPath)
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if g.ExecutableFallback.Path == "" {
			return ExecutableInfo{}, fmt.Errorf("game: no %s in %s and no fallback executable configured for %s", g.LauncherSettingsPath, installDir, g.ID)
		}
		return withWorkingDir(g.ExecutableFallback), nil
	}

	var ls launcherSettings
	if err := json.Unmarshal(data, &ls); err != nil {
		if g.ExecutableFallback.Path == "" {
			return ExecutableInfo{}, fmt.Errorf("game: malformed launcher-settings.json in %s: %w", installDir, err)
		}
		return withWorkingDir(g.ExecutableFallback), nil
	}
	if ls.ExePath == "" {
		if g.ExecutableFallback.Path == "" {
			return ExecutableInfo{}, fmt.Errorf("game: launcher-settings.json in %s has no exePath and no fallback executable configured", installDir)
		}
		return withWorkingDir(g.ExecutableFallback), nil
	}

	exePath := filepath.Clean(filepath.Join(filepath.Dir(settingsPath), ls.ExePath))
	return ExecutableInfo{Path: exePath, Args: ls.ExeArgs, WorkingDir: filepath.Dir(exePath)}, nil
}

// withWorkingDir fills in info.WorkingDir from info.Path when the caller (a
// games.jsonc ExecutableFallback entry) didn't already set one of its own.
func withWorkingDir(info ExecutableInfo) ExecutableInfo {
	if info.WorkingDir == "" {
		info.WorkingDir = filepath.Dir(info.Path)
	}
	return info
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
