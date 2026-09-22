package checksum

import (
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A checksum is asked for again on every playset save and load, and whenever mod files change on
// disk (see PlaysetChecksum) - in the overwhelmingly common case, nothing on disk actually
// changed since the last call. Reading and hashing every game and mod file again to find that out
// is real, measured work: about 80ms for a small Stellaris playset over 2,332 files on the machine
// this was developed on, and it only grows with a bigger mod list. ResultCache is what lets that
// be skipped: a fingerprint of every file's path, size and modification time is built from the same
// walk Compute already does to find the files (nothing extra is read from disk for it), and only a
// fingerprint that differs from last time falls through to the real, full recomputation - matching
// this project's cache-verify-on-read rule elsewhere (internal/cache): a cache hit is never
// trusted without checking it first, and any real change (a file added, removed, edited, or the
// enabled mods themselves changing) is guaranteed to change the fingerprint and so is never missed.
//
// The zero value is ready to use and safe for concurrent use from multiple goroutines. Nothing is
// persisted to disk: an empty cache after a restart just means the first call of the new run pays
// the real cost once, same as before this existed.
type ResultCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	order   []string // keys, oldest first, for eviction
}

type cacheEntry struct {
	fingerprint string
	result      Result
}

// maxCachedResults bounds how many distinct game+mod-set combinations are remembered at once -
// enough for switching between a handful of playsets without each evicting the last, small enough
// that the cache can never grow without bound over a long session.
const maxCachedResults = 8

// cacheKey identifies which game and which set of enabled mods, in order, a Result is for -
// everything Compute's answer depends on other than the files' own content, which the fingerprint
// covers separately.
func cacheKey(in Input) string {
	var b strings.Builder
	b.WriteString(string(in.Algorithm))
	b.WriteByte('\n')
	b.WriteString(in.GameDir)
	b.WriteByte('\n')
	b.WriteString(in.ModDir)
	b.WriteByte('\n')
	for _, m := range in.Mods {
		b.WriteString(m.ID)
		b.WriteByte('\x1f')
	}
	return b.String()
}

// fingerprintOf hashes salt together with every file's path, size and modification time, in the
// order given (already the deterministic order Compute itself would hash them in, though the
// fingerprint does not depend on that: any consistent order gives the same answer for the same
// files). get reads those three things out of one item without either algorithm's own item type
// leaking into this file.
func fingerprintOf[T any](salt string, items []T, get func(T) (path string, size int64, modTime time.Time)) string {
	h := fnv.New64a()
	h.Write([]byte(salt))
	h.Write([]byte{0})
	var buf [8]byte
	for _, it := range items {
		path, size, modTime := get(it)
		h.Write([]byte(path))
		h.Write([]byte{0})
		putUint64(&buf, uint64(size))
		h.Write(buf[:])
		putUint64(&buf, uint64(modTime.UnixNano()))
		h.Write(buf[:])
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

func putUint64(buf *[8]byte, v uint64) {
	for i := 0; i < 8; i++ {
		buf[i] = byte(v >> (8 * i))
	}
}

// lookup returns the Result cached under key, but only if fingerprint matches what produced it. A
// nil receiver (no cache configured) always misses.
func (c *ResultCache) lookup(key, fingerprint string) (Result, bool) {
	if c == nil {
		return Result{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || e.fingerprint != fingerprint {
		return Result{}, false
	}
	return e.result.clone(), true
}

// store remembers result under key, keyed to fingerprint, evicting the oldest entry once the
// cache is full. A nil receiver is a no-op, so a caller that chose not to configure a cache never
// has to check for one before calling this.
func (c *ResultCache) store(key, fingerprint string, result Result) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]cacheEntry{}
	}
	if _, existed := c.entries[key]; !existed {
		c.order = append(c.order, key)
		if len(c.order) > maxCachedResults {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.entries[key] = cacheEntry{fingerprint: fingerprint, result: result.clone()}
}

// clone returns a copy of r that shares no slice with it, so neither a cached Result nor one just
// handed to a caller can be mutated through the other.
func (r Result) clone() Result {
	r.Order = append([]string(nil), r.Order...)
	r.Warnings = append([]string(nil), r.Warnings...)
	return r
}
