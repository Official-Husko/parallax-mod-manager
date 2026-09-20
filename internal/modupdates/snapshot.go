// Package modupdates answers "what happened to my mods since I last started the
// app": which were updated, which had their files changed, which are gone from
// the disk, and which were deleted from the Steam Workshop.
//
// Every startup records what each mod looked like (its version, a fingerprint of
// its files, and what the Workshop said about it), and the next startup diffs
// against that record. Nothing here talks to Steam or reads the mod list
// itself: the caller hands over what it found, which keeps the diff a pure
// function that can be tested with plain values.
package modupdates

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Fingerprint is a cheap summary of a mod's content: enough to notice that
// something in it was added, removed or rewritten, without hashing anything.
type Fingerprint struct {
	// Known is false when the content could not be read (its folder is missing,
	// or a Workshop download has not finished): such a fingerprint is never
	// compared, so it can neither report a change nor hide one.
	Known bool `json:"known"`
	// Files is how many files the mod holds.
	Files int `json:"files"`
	// Size is their total size in bytes.
	Size int64 `json:"size"`
	// Newest is the most recent modified time among them, in unix seconds.
	Newest int64 `json:"newest"`
}

// Record is what one mod looked like at one moment.
type Record struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version"`
	// RemoteFileID is the mod's Steam Workshop item id (Workshop mods only).
	RemoteFileID string      `json:"remoteFileId,omitempty"`
	Content      Fingerprint `json:"content"`
	// WorkshopUpdated is when the Workshop page was last updated, in unix
	// seconds; 0 when it is not known (not a Workshop mod, or Steam could not be
	// asked).
	WorkshopUpdated int64 `json:"workshopUpdated"`
	// WorkshopGone is true when Steam reports the item as deleted or banned.
	WorkshopGone bool `json:"workshopGone"`
	// GoneSince is when the item was first seen gone, in unix seconds.
	GoneSince int64 `json:"goneSince"`
}

// Snapshot is every mod of one game at one moment, keyed by mod id.
type Snapshot struct {
	TakenAt int64             `json:"takenAt"`
	Mods    map[string]Record `json:"mods"`
}

// header explains the file to anyone who opens it - written as JSONC (see
// internal/jsonc), so the comments survive next to the data they describe.
const header = `// Parallax Mod Manager: what this game's mods looked like the last time the app ran.
// The next startup compares against this to report which mods were updated,
// changed, removed or deleted from the Steam Workshop in between. It is safe to
// delete: the next startup then has nothing to compare with and starts a new one.
//
// takenAt: when this was written, in unix seconds.
// mods: one entry per mod, keyed by the mod's id, each with:
//   name, source (local, workshop or paradox-launcher), version: from its descriptor.
//   remoteFileId: the Steam Workshop item id (Workshop mods only).
//   content: a fingerprint of the mod's files - known (false when they could not
//     be read), files (how many), size (total bytes), newest (latest modified
//     time, unix seconds).
//   workshopUpdated: when the Workshop page was last updated, unix seconds (0 =
//     unknown).
//   workshopGone: true when Steam says the item was deleted or banned.
//   goneSince: when the app first saw it gone, unix seconds.
`

// Store keeps one Snapshot per game on disk. Dir is the folder they live in;
// left empty, nothing is loaded and saving fails (the config folder could not
// be resolved), which callers treat as "no history" rather than a fatal error.
type Store struct {
	Dir string
}

func (s Store) filename(gameID string) string {
	return gameID + ".jsonc"
}

// Load returns gameID's saved snapshot, or nil when there is none or it cannot
// be read - a missing or corrupt file just means there is nothing to compare
// against yet, the same degrade-to-empty rule as internal/versionignore.
func (s Store) Load(gameID string) *Snapshot {
	if s.Dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, s.filename(gameID)))
	if err != nil {
		return nil
	}
	var snap Snapshot
	if err := jsonc.Unmarshal(data, &snap); err != nil || snap.Mods == nil {
		return nil
	}
	return &snap
}

// Save atomically writes gameID's snapshot, replacing the last one.
func (s Store) Save(gameID string, snap Snapshot) error {
	if s.Dir == "" {
		return fmt.Errorf("modupdates: Store dir must be set explicitly")
	}
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("modupdates: encoding snapshot for %q: %w", gameID, err)
	}
	data := append([]byte(header), body...)
	data = append(data, '\n')
	if _, err := atomicfile.Write(s.Dir, s.filename(gameID), data); err != nil {
		return fmt.Errorf("modupdates: saving snapshot for %q: %w", gameID, err)
	}
	return nil
}
