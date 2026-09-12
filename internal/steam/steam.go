// Package steam locates Steam Workshop content across a user's Steam
// libraries. A user can have multiple Steam libraries across multiple
// drives, so finding a game's Workshop folder requires first reading Steam's
// own libraryfolders.vdf. See docs/mod-sources.md.
package steam

import (
	"fmt"
	"os"
	"path/filepath"
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
