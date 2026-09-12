// Package cache implements the incremental stat -> hash -> parse cache
// described in docs/performance-strategy.md: the single biggest lever this
// project has for avoiding the full re-parse most existing mod managers do
// on every launch. Compare cheap (mtime, size) first; only hash if that
// differs; only re-parse if the hash differs. Results persist to disk, one
// JSON file per mod, so a relaunch with an unchanged mod list approaches
// "nothing to do" instead of a full re-scan.
package cache

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// FormatVersion guards the on-disk cache file shape. Bump it whenever
// FileRecord or ModCache's structure changes incompatibly; Load fails closed
// (treats the file as absent) on a mismatch rather than risk decoding a
// record in a shape it no longer understands.
const FormatVersion = 1

// FileRecord is one mod file's cached state.
type FileRecord struct {
	Path            string                  `json:"path"` // mod-relative
	ModTimeUnixNano int64                   `json:"mtime"`
	Size            int64                   `json:"size"`
	Hash            uint64                  `json:"hash"` // xhash.Bytes of the raw file content
	Definitions     []definition.Definition `json:"definitions"`
	// ParseError, when non-empty, sticks the file's last-known-bad status to
	// its stat+hash: it isn't re-attempted every run until it actually
	// changes on disk, but a change always gets a fresh try.
	ParseError string `json:"parseError,omitempty"`
}

// ModCache is one mod's complete cached state.
type ModCache struct {
	Version int                   `json:"version"`
	ModID   string                `json:"modId"`
	GameKey string                `json:"gameKey"`
	Files   map[string]FileRecord `json:"files"`
}

func newModCache(gameKey, modID string) *ModCache {
	return &ModCache{Version: FormatVersion, ModID: modID, GameKey: gameKey, Files: map[string]FileRecord{}}
}

// Lookup checks a file's cached record against its current stat info. It
// does no I/O beyond the fs.FileInfo the caller already has - the whole
// point is to make the common case (nothing changed) as cheap as possible.
func (c *ModCache) Lookup(relPath string, info fs.FileInfo) (FileRecord, bool) {
	rec, ok := c.Files[relPath]
	if !ok {
		return FileRecord{}, false
	}
	if rec.ModTimeUnixNano != info.ModTime().UnixNano() || rec.Size != info.Size() {
		return FileRecord{}, false
	}
	return rec, true
}

// Put records (or replaces) one file's cached state.
func (c *ModCache) Put(relPath string, rec FileRecord) {
	c.Files[relPath] = rec
}

// Store persists ModCache records.
type Store interface {
	// Load never errors just because no cache exists yet for this
	// (gameKey, modID) - it returns a fresh, empty ModCache in that case.
	Load(ctx context.Context, gameKey, modID string) (*ModCache, error)
	Save(ctx context.Context, c *ModCache) error
}

// FileStore is the on-disk Store: one JSON file per mod, at
// <Dir>/<gameKey>/<modID>.json, written atomically (temp file + rename) so a
// crash mid-write can never leave a half-written, corrupt cache file.
type FileStore struct {
	Dir string
}

func (s FileStore) path(gameKey, modID string) string {
	return filepath.Join(s.Dir, gameKey, modID+".json")
}

// Load reads one mod's cache file. A missing file, a JSON decode error, or a
// Version mismatch all fail closed to a fresh empty ModCache - the caller
// re-parses that one mod from scratch, nothing crashes, and no partially- or
// incorrectly-decoded state is ever propagated. This is deliberately
// forgiving: research into this problem space turned up a real bug of
// exactly this shape (a cache silently trusting bad state) elsewhere,
// worth designing against from the start (see docs/performance-strategy.md).
func (s FileStore) Load(ctx context.Context, gameKey, modID string) (*ModCache, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.path(gameKey, modID))
	if err != nil {
		return newModCache(gameKey, modID), nil
	}

	var c ModCache
	if err := json.Unmarshal(data, &c); err != nil {
		return newModCache(gameKey, modID), nil
	}
	if c.Version != FormatVersion {
		return newModCache(gameKey, modID), nil
	}
	if c.Files == nil {
		c.Files = map[string]FileRecord{}
	}
	return &c, nil
}

// Save atomically writes c to disk.
func (s FileStore) Save(ctx context.Context, c *ModCache) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(s.Dir, c.GameKey)
	_, err := atomicfile.WriteJSON(dir, c.ModID+".json", c)
	return err
}
