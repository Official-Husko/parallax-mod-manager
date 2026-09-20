// Package cache implements the incremental stat -> hash -> parse cache
// described in docs/performance-strategy.md: the single biggest lever this
// project has for avoiding the full re-parse most existing mod managers do
// on every launch. Compare cheap (mtime, size) first; only hash if that
// differs; only re-parse if the hash differs. Results persist to disk, one
// gob file per mod, so a relaunch with an unchanged mod list approaches
// "nothing to do" instead of a full re-scan.
//
// Encoding: gob, not JSON. A content-heavy mod (a total conversion with
// thousands of files) can carry tens of thousands of cached Definitions,
// and encoding/json's per-field name repetition and reflection-heavy
// (de)serialization measurably dominates Load/Save wall time at that scale -
// confirmed on a real ~1,800-file Workshop mod, where the JSON cache file
// reached 165MB and Load/Save alone cost over a second combined, on every
// single call regardless of how many files were actually unchanged. Gob's
// self-describing-once-per-type wire format measured roughly half the file
// size and 3-5x faster encode/decode on that same real data - the fix that
// actually makes "unchanged mods cost near-zero on relaunch" true at this
// scale, not just for small mod lists.
package cache

import (
	"bytes"
	"context"
	"encoding/gob"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// FormatVersion guards the on-disk cache file shape. Bump it whenever
// FileRecord or ModCache's structure changes incompatibly (3: FileRecord got
// its own compact encoding, see record_codec.go); Load fails closed
// (treats the file as absent) on a mismatch rather than risk decoding a
// record in a shape it no longer understands.
const FormatVersion = 3

// ParserVersion identifies what the parsers produced when a mod's cache was
// written. Bump it whenever internal/script, internal/locale or
// internal/definition (including how a definition is normalized and hashed)
// change what they return for the same file - a fix that lets a file parse
// that used to fail, say. A cached file is remembered by its size and mtime
// alone, so without this a fixed parser would never get a second look at a
// file it once rejected: its "failed" verdict would sit in the cache until the
// file itself changed. A cache written under a different version is treated as
// absent, and everything in that mod is parsed again once.
//
// 1: locale values may be followed by a "#" comment; repeated BOMs are stripped.
// 2: a malformed localisation line is skipped instead of failing the whole file.
const ParserVersion = 2

// cacheFileExt is the on-disk extension for FileStore's gob-encoded cache
// files - deliberately distinct from the old JSON-format ".json" extension
// this package used before FormatVersion 2, so a leftover pre-upgrade cache
// file is simply never found (Load treats it as no cache yet) rather than
// risk gob-decoding bytes that were never gob to begin with.
const cacheFileExt = ".gobcache"

// FileRecord is one mod file's cached state.
type FileRecord struct {
	Path            string // mod-relative
	ModTimeUnixNano int64
	Size            int64
	Hash            uint64 // xhash.Bytes of the raw file content
	Definitions     []definition.Definition
	// ParseError, when non-empty, sticks the file's last-known-bad status to
	// its stat+hash: it isn't re-attempted every run until it actually
	// changes on disk, but a change always gets a fresh try.
	ParseError string
}

// ModCache is one mod's complete cached state.
type ModCache struct {
	Version int
	// ParserVersion is the ParserVersion this cache was written under. A file
	// written before the field existed decodes it as 0, which never matches,
	// so those are all rebuilt once.
	ParserVersion int
	ModID         string
	GameKey       string
	Files         map[string]FileRecord
}

func newModCache(gameKey, modID string) *ModCache {
	return &ModCache{Version: FormatVersion, ParserVersion: ParserVersion, ModID: modID, GameKey: gameKey, Files: map[string]FileRecord{}}
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

// FileStore is the on-disk Store: one gob file per mod, at
// <Dir>/<gameKey>/<modID>.gobcache, written atomically (temp file + rename)
// so a crash mid-write can never leave a half-written, corrupt cache file.
type FileStore struct {
	Dir string
}

func (s FileStore) path(gameKey, modID string) string {
	return filepath.Join(s.Dir, gameKey, modID+cacheFileExt)
}

// Load reads one mod's cache file. A missing file, a gob decode error, or a
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
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&c); err != nil {
		return newModCache(gameKey, modID), nil
	}
	if c.Version != FormatVersion || c.ParserVersion != ParserVersion {
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
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	dir := filepath.Join(s.Dir, c.GameKey)
	_, err := atomicfile.Write(dir, c.ModID+cacheFileExt, buf.Bytes())
	return err
}
