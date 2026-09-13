// Package dlc discovers a classic-descriptor game's real installed DLC by
// scanning its install directory - see docs/game-launching.md for the
// on-disk shape, confirmed against a real Stellaris install: each DLC is
// its own folder under "<InstallDir>/dlc/" (e.g. "dlc013_horizon_signal")
// containing one ".dlc" metadata file, plain Clausewitz key-value text
// (the same format internal/mod's classic descriptor parser already
// handles), with at least a "name" field. The folder name itself - not the
// display name - is the exact string dlc_load.json's "disabled_dlcs" list
// uses.
package dlc

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

// Entry is one real, installed DLC.
type Entry struct {
	// ID is the dlc/ subfolder's own name - what disabled_dlcs identifies
	// it by, not something this project invents.
	ID string
	// Name is the .dlc file's own "name" field.
	Name string
	// Category is the .dlc file's own "category" field (e.g.
	// "story_pack", "expansion", "content_pack"), if it has one.
	Category string
	// SteamID is the .dlc file's own "steam_id" field - the real Steam
	// Store app id for this DLC, confirmed against a real Stellaris
	// install. This is the bridge internal/dlcstore uses to fetch this
	// exact DLC's Store listing (header image, description) without
	// needing to guess a mapping between the two identifier spaces.
	// Empty if the .dlc file didn't declare one.
	SteamID string
	// SizeBytes sums every real file under this DLC's own folder -
	// confirmed meaningful, not a placeholder, against a real Stellaris
	// install: a free content pack's folder is genuinely only tens of KB
	// (metadata and a thumbnail), while a full paid expansion's is tens
	// of megabytes of real archived content. Steam's own public Store API
	// has no size field to fetch instead - this is the only real source
	// for it.
	SizeBytes int64
	// Installed is true for every Entry Discover actually finds locally.
	// library.MergeDLCCatalog adds synthetic Entry values with this false
	// for DLC the base game's own Steam Store catalog lists that aren't
	// installed here - real to show (name, header image via
	// internal/dlcstore), but never toggleable, since there's no local
	// folder for disabled_dlcs to reference.
	Installed bool
}

// Discover finds every real DLC installed under installDir's dlc/
// subfolder. A missing dlc/ folder (the game has none, or isn't installed)
// is not an error - it just means no DLC exists yet, matching this
// project's "missing is fine, don't fabricate" handling elsewhere (see
// scan.Scan). A single unreadable or malformed .dlc file is skipped rather
// than failing the whole discovery.
func Discover(installDir string) ([]Entry, error) {
	dlcDir := filepath.Join(installDir, "dlc")
	folders, err := os.ReadDir(dlcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	found := []Entry{}
	for _, f := range folders {
		if !f.IsDir() {
			continue
		}
		dir := filepath.Join(dlcDir, f.Name())
		entry, ok := readEntry(dir, f.Name())
		if !ok {
			continue
		}
		entry.SizeBytes = dirSize(dir)
		found = append(found, entry)
	}
	return found, nil
}

// readEntry parses dir's one ".dlc" metadata file for its "name" and
// "category" fields. ok is false when dir has no readable, parseable .dlc
// file at all - the caller skips it rather than fabricating an entry.
func readEntry(dir, id string) (entry Entry, ok bool) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return Entry{}, false
	}
	for _, f := range files {
		if f.IsDir() || !strings.EqualFold(filepath.Ext(f.Name()), ".dlc") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		parsed, err := script.Parse(data)
		if err != nil {
			continue
		}
		entry = Entry{ID: id, Installed: true}
		for _, e := range parsed.Root.Entries {
			switch e.Key {
			case "name":
				entry.Name = e.Value.Raw
			case "category":
				entry.Category = e.Value.Raw
			case "steam_id":
				entry.SteamID = e.Value.Raw
			}
		}
		if entry.Name == "" {
			continue // no usable name - not a real, displayable DLC entry
		}
		return entry, true
	}
	return Entry{}, false
}

// dirSize sums every real file's size under root. Errors are swallowed -
// a DLC folder that can't be fully walked still gets a real (if partial)
// entry rather than none at all; this is a display figure, not something
// anything correctness-critical depends on.
func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
