package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// IndexFile is the name of the per-game record of what was copied.
const IndexFile = "backups.jsonc"

// Entry is one mod's copy.
type Entry struct {
	// ModID is the app's id for the mod (for showing); RemoteFileID names the folder.
	ModID        string
	RemoteFileID string
	Name         string
	Version      string
	// BackedUpAt is when the copy was made (Unix seconds).
	BackedUpAt int64
	// Reason is why it was copied: "deleted", "private", "all" or "manual".
	Reason string
	// Files and Size describe the copy (for a complete one, the source it matches).
	Files int
	Size  int64
	// Newest is the newest modification time in the source when copied, which with Files
	// and Size tells whether the mod has changed since.
	Newest int64
	// Complete is false when files vanished while copying: what is there is what could
	// be saved, and Missing says how many files could not be.
	Complete bool
	Missing  int
}

// index is the file's content.
type index struct {
	Version int              `json:"version"`
	Mods    map[string]Entry `json:"mods"`
}

const indexVersion = 1

// indexMu serialises reading, changing and writing index files: copies for different
// mods can finish at the same moment.
var indexMu sync.Mutex

func indexPath(gameDir string) string { return filepath.Join(gameDir, IndexFile) }

// loadIndex reads a game's index; a missing or unreadable one is an empty index (the
// copies themselves are still on disk, only the record of them is gone).
func loadIndex(gameDir string) index {
	idx := index{Version: indexVersion, Mods: map[string]Entry{}}
	data, err := os.ReadFile(indexPath(gameDir))
	if err != nil {
		return idx
	}
	var read index
	if err := jsonc.Unmarshal(data, &read); err != nil || read.Mods == nil {
		return idx
	}
	return read
}

const indexHeader = `// Parallax Mod Manager - what was backed up for this game.
//
// The mod copies are in the "mods" folder next to this file, one folder per Workshop
// item, copied as they were. This file only records what each one is and why it was
// copied: it can be deleted without harming the copies.
`

// recordEntry adds or replaces one mod's entry.
func recordEntry(gameDir string, e Entry) error {
	indexMu.Lock()
	defer indexMu.Unlock()
	idx := loadIndex(gameDir)
	idx.Version = indexVersion
	idx.Mods[e.RemoteFileID] = e
	return writeIndex(gameDir, idx)
}

func writeIndex(gameDir string, idx index) error {
	body, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	data := append([]byte(indexHeader), body...)
	data = append(data, '\n')
	_, err = atomicfile.Write(gameDir, IndexFile, data)
	return err
}

// List returns a game's backups, newest first. Entries whose folder has been deleted
// by hand are left out.
func List(root, gameID string) []Entry {
	if root == "" || gameID == "" || strings.ContainsAny(gameID, `/\`) {
		return nil
	}
	gameDir := filepath.Join(root, gameID)
	indexMu.Lock()
	idx := loadIndex(gameDir)
	indexMu.Unlock()
	out := make([]Entry, 0, len(idx.Mods))
	for id, e := range idx.Mods {
		if _, err := os.Stat(filepath.Join(gameDir, "mods", id)); err != nil {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BackedUpAt != out[j].BackedUpAt {
			return out[i].BackedUpAt > out[j].BackedUpAt
		}
		return out[i].RemoteFileID < out[j].RemoteFileID
	})
	return out
}

// Lookup returns one mod's entry, if it is backed up.
func Lookup(root, gameID, remoteFileID string) (Entry, bool) {
	for _, e := range List(root, gameID) {
		if e.RemoteFileID == remoteFileID {
			return e, true
		}
	}
	return Entry{}, false
}

// FolderFor is where a mod's copy lives.
func FolderFor(root, gameID, remoteFileID string) string {
	return filepath.Join(root, gameID, "mods", remoteFileID)
}

// GameFolder is where a game's copies and index live.
func GameFolder(root, gameID string) string { return filepath.Join(root, gameID) }

// FreeSpace is the bytes free on the volume path is (or would be) on: the nearest
// folder that exists is asked. false when it cannot be found out.
func FreeSpace(path string) (uint64, bool) {
	for p := path; ; {
		if _, err := os.Stat(p); err == nil {
			return freeBytes(p)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return 0, false
		}
		p = parent
	}
}

// TotalSize is what every game's backups under root take together, as recorded (a copy
// whose folder was deleted by hand is not counted).
func TotalSize(root string) int64 {
	if root == "" {
		return 0
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	var total int64
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		for _, e := range List(root, d.Name()) {
			total += e.Size
		}
	}
	return total
}

// Deleted says what Delete removed.
type Deleted struct {
	// IDs are the Workshop items whose copies were removed.
	IDs []string
	// Bytes is the space they took, as recorded.
	Bytes int64
}

// Delete removes the copies of the given Workshop items from a game's backups, and
// their records. An id that is not a Workshop item id is refused (it names a folder, so
// it must never point outside the backup folder); one that has no copy is skipped. The
// first failure is returned after the rest were tried.
func Delete(root, gameID string, itemIDs []string) (Deleted, error) {
	var out Deleted
	if root == "" || gameID == "" || strings.ContainsAny(gameID, `/\`) || gameID == "." || gameID == ".." {
		return out, errors.New("backup: no such game folder")
	}
	gameDir := filepath.Join(root, gameID)
	indexMu.Lock()
	defer indexMu.Unlock()
	idx := loadIndex(gameDir)
	var firstErr error
	for _, id := range itemIDs {
		if !itemIDPattern.MatchString(id) {
			if firstErr == nil {
				firstErr = fmt.Errorf("backup: %q is not a Workshop item id", id)
			}
			continue
		}
		dir := FolderFor(root, gameID, id)
		_, statErr := os.Stat(dir)
		entry, recorded := idx.Mods[id]
		if statErr != nil && !recorded {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("backup: removing %s: %w", dir, err)
			}
			continue
		}
		_ = os.RemoveAll(dir + ".partial")
		_ = os.RemoveAll(dir + ".old")
		if recorded {
			out.Bytes += entry.Size
			delete(idx.Mods, id)
		}
		out.IDs = append(out.IDs, id)
	}
	if len(out.IDs) > 0 {
		if err := writeIndex(gameDir, idx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("backup: updating the record of backups: %w", err)
		}
	}
	return out, firstErr
}
