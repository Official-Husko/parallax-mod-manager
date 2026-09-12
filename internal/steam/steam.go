// Package steam locates Steam Workshop content across a user's Steam
// libraries. A user can have multiple Steam libraries across multiple
// drives, so finding a game's Workshop folder requires first reading Steam's
// own libraryfolders.vdf. See docs/mod-sources.md.
package steam

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
)

// LibraryFolder is one Steam library location, with the set of app ids
// installed in it.
type LibraryFolder struct {
	Path string
	Apps map[string]struct{}
}

// ParseLibraryFolders parses Steam's steamapps/libraryfolders.vdf, which
// lists every configured Steam library and the app ids installed in each.
// It's Valve's own small key/value format (VDF), not Clausewitz script.
func ParseLibraryFolders(vdfPath string) ([]LibraryFolder, error) {
	data, err := os.ReadFile(vdfPath)
	if err != nil {
		return nil, fmt.Errorf("steam: reading %s: %w", vdfPath, err)
	}
	root, err := parseVDF(data)
	if err != nil {
		return nil, fmt.Errorf("steam: parsing %s: %w", vdfPath, err)
	}

	// Expected shape:
	// "libraryfolders" {
	//   "0" { "path" "C:\\Steam" "apps" { "281990" "123456789" ... } }
	//   "1" { ... }
	// }
	top, ok := root["libraryfolders"]
	if !ok {
		return nil, fmt.Errorf("steam: %s has no top-level \"libraryfolders\" key", vdfPath)
	}

	// Library entries are keyed "0", "1", "2", ... but map iteration order
	// is randomized in Go - sort numerically so results are deterministic.
	keys := make([]string, 0, len(top.children))
	for k := range top.children {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, ierr := strconv.Atoi(keys[i])
		nj, jerr := strconv.Atoi(keys[j])
		if ierr == nil && jerr == nil {
			return ni < nj
		}
		return keys[i] < keys[j]
	})

	var libraries []LibraryFolder
	for _, key := range keys {
		entry := top.children[key]
		path := entry.values["path"]
		if path == "" {
			continue
		}
		lib := LibraryFolder{Path: path, Apps: map[string]struct{}{}}
		if apps, ok := entry.children["apps"]; ok {
			for appID := range apps.values {
				lib.Apps[appID] = struct{}{}
			}
		}
		libraries = append(libraries, lib)
	}
	return libraries, nil
}

// FindWorkshopContentDir searches every library under steamRoot's
// libraryfolders.vdf for one that has appID installed, and returns that
// library's Workshop content directory for the game
// (<library>/steamapps/workshop/content/<appID>).
func FindWorkshopContentDir(steamRoot, appID string) (string, error) {
	vdfPath := filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf")
	libraries, err := ParseLibraryFolders(vdfPath)
	if err != nil {
		return "", err
	}
	for _, lib := range libraries {
		if _, ok := lib.Apps[appID]; !ok {
			continue
		}
		dir := filepath.Join(lib.Path, "steamapps", "workshop", "content", appID)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, nil
		}
	}
	return "", fmt.Errorf("steam: no Steam library with app %s and an existing workshop content folder found under %s", appID, steamRoot)
}

// DefaultRoots returns every well-known Steam installation root for the
// current OS that actually exists on disk. A user's game libraries
// themselves (a secondary drive, say) don't need to be listed here - each
// candidate's own steamapps/libraryfolders.vdf already enumerates every
// library Steam knows about, main install included.
func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var candidates []string
	switch runtime.GOOS {
	case "linux":
		candidates = []string{
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
		}
	case "darwin":
		candidates = []string{filepath.Join(home, "Library", "Application Support", "Steam")}
	default: // windows
		candidates = []string{
			`C:\Program Files (x86)\Steam`,
			`C:\Program Files\Steam`,
		}
	}

	var roots []string
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			roots = append(roots, c)
		}
	}
	return roots
}

// FindGameInstallDir searches every library under every default Steam root
// for a folderName directory under steamapps/common, the same
// exists-on-disk-not-just-in-bookkeeping check FindWorkshopContentDir uses -
// a game can be removed from disk without libraryfolders.vdf being updated.
func FindGameInstallDir(appID string) (string, error) {
	for _, root := range DefaultRoots() {
		// The root's own steamapps/common counts too, whether or not its
		// libraryfolders.vdf happens to list itself as a library.
		libraryPaths := []string{root}
		if libs, err := ParseLibraryFolders(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, lib := range libs {
				libraryPaths = append(libraryPaths, lib.Path)
			}
		}
		for _, libPath := range libraryPaths {
			installDirName, err := readAppManifestInstallDir(libPath, appID)
			if err != nil {
				continue
			}
			dir := filepath.Join(libPath, "steamapps", "common", installDirName)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir, nil
			}
		}
	}
	return "", fmt.Errorf("steam: no installed copy of app %s found under any default Steam library", appID)
}

// readAppManifestInstallDir reads libraryPath's own Steam app manifest for
// appID (steamapps/appmanifest_<appID>.acf - the same VDF format as
// libraryfolders.vdf, despite the .acf extension) and returns its real
// installdir field. This is the authoritative folder name Steam itself
// installed the game under - a game's display name, its Paradox user-data
// folder name (GameConfig.FolderName), and this can all differ, so this
// must never be guessed from any of those.
func readAppManifestInstallDir(libraryPath, appID string) (string, error) {
	acfPath := filepath.Join(libraryPath, "steamapps", fmt.Sprintf("appmanifest_%s.acf", appID))
	data, err := os.ReadFile(acfPath)
	if err != nil {
		return "", err
	}
	root, err := parseVDF(data)
	if err != nil {
		return "", fmt.Errorf("steam: parsing %s: %w", acfPath, err)
	}
	appState, ok := root["AppState"]
	if !ok {
		return "", fmt.Errorf("steam: %s has no top-level \"AppState\" key", acfPath)
	}
	installDir := appState.values["installdir"]
	if installDir == "" {
		return "", fmt.Errorf("steam: %s has no installdir", acfPath)
	}
	return installDir, nil
}
