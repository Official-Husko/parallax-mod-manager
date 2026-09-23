// Package loverslabmeta is a small, partial, on-disk cache of per-file
// details this app already fetches for other real reasons (a file's own
// real author avatar, view count, and updated date - see
// loverslab.FileDetail) but the Browse tab's own browsing-grid listing page
// never has room for at all: confirmed live, LoversLab's real category
// listing shows only a plain author-name link and a downloads count per
// card, nothing else - no avatar image, no date, no view count anywhere in
// its own markup.
//
// This is deliberately never fetched just to fill itself in - every entry
// here is a side effect of a real loverslab.FileDetail fetch that already
// happened for its own reason (opening a mod's own detail view; see
// internal/app/loverslab.go's cacheFileMeta). That's what makes it
// "partial": a file this app has simply never looked at yet has no entry
// at all, and the browsing grid just shows what it always has for one (a
// hashed initial letter, no date, no views) - never a blocking fetch, never
// a guess. Once cached, an entry is reused indefinitely rather than
// re-fetched or expired: an author's own avatar and a file's own upload
// date essentially never change, so there is no real staleness risk here
// the way there is for, say, a category listing's own page cache
// (internal/loverslab/client.go's separate, short-lived getDocument cache).
package loverslabmeta

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// maxEntries is a soft cap on how large this cache is ever allowed to grow -
// a real, long-running install could otherwise accumulate one entry per
// distinct file ever browsed, indefinitely. With evicts the oldest (by
// CachedAt) tenth of the map once a new entry would push it over this, so
// the file stays bounded without needing a person to ever clear it by hand.
const maxEntries = 4000

// Entry is what this app has separately, opportunistically learned about
// one LoversLab file beyond what its own listing-page card ever shows.
type Entry struct {
	// AuthorAvatarURL is the file's own author's profile photo (see
	// loverslab.FileDetail.Author.ImageURL) - "" for an author with no
	// avatar set, same as everywhere else this app reads one.
	AuthorAvatarURL string `json:"authorAvatarUrl"`
	// Views is the file's own real view count at the time it was cached
	// (loverslab.FileDetail.Views) - a point-in-time snapshot, not
	// refreshed until this file's detail page happens to be fetched again.
	Views int `json:"views"`
	// Updated is the site's own real "dateModified" ISO 8601 timestamp
	// (loverslab.FileDetail.DateModified) - distinct from, and far more
	// often populated than, loverslab.FileSummary's own Updated (the
	// listing page's own display string, which the real site stopped
	// rendering at all - see docs/loverslab.md).
	Updated string `json:"updated"`
	// CachedAt is when this entry was last written (unix seconds) - not
	// used to expire an entry (see the package comment on why that's not
	// needed here), only to decide which entries are oldest if this cache
	// ever needs to evict some to stay under maxEntries.
	CachedAt int64 `json:"cachedAt"`
}

// Store keeps this cache on disk as one file in Dir.
type Store struct {
	Dir string
}

func (s Store) path() string {
	return filepath.Join(s.Dir, "cache.jsonc")
}

// Load returns every cached entry, by LoversLab file ID. No file at all
// means no entries yet, not an error - this cache starts genuinely empty on
// a fresh install and fills in gradually.
func (s Store) Load() (map[int]Entry, error) {
	if s.Dir == "" {
		return map[int]Entry{}, nil
	}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return map[int]Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loverslabmeta: reading %s: %w", s.path(), err)
	}
	var f struct {
		Files map[string]Entry `json:"files"`
	}
	if err := jsonc.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("loverslabmeta: %s is not valid (fix or remove it by hand; nothing was changed): %w", s.path(), err)
	}
	entries := make(map[int]Entry, len(f.Files))
	for idStr, e := range f.Files {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		entries[id] = e
	}
	return entries, nil
}

// Save writes every cached entry atomically, replacing the file.
func (s Store) Save(entries map[int]Entry) error {
	if s.Dir == "" {
		return errors.New("loverslabmeta: there is no settings folder to cache file details in")
	}
	if _, err := atomicfile.Write(s.Dir, filepath.Base(s.path()), render(entries)); err != nil {
		return fmt.Errorf("loverslabmeta: saving: %w", err)
	}
	return nil
}

// With returns entries with fileID's own entry set to e, evicting the
// oldest tenth of the map first if adding it would push the map over
// maxEntries. entries itself is not modified.
func With(entries map[int]Entry, fileID int, e Entry) map[int]Entry {
	out := make(map[int]Entry, len(entries)+1)
	for id, existing := range entries {
		out[id] = existing
	}
	if _, exists := out[fileID]; !exists && len(out) >= maxEntries {
		evictOldest(out, len(out)/10+1)
	}
	out[fileID] = e
	return out
}

func evictOldest(entries map[int]Entry, count int) {
	type idAge struct {
		id  int
		age int64
	}
	ids := make([]idAge, 0, len(entries))
	for id, e := range entries {
		ids = append(ids, idAge{id, e.CachedAt})
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].age < ids[j].age })
	for i := 0; i < count && i < len(ids); i++ {
		delete(entries, ids[i].id)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// render writes the file: a comment saying what it is, then one entry per
// line in ID order so the file stays easy to read and diff, same convention
// as internal/loverslabtracking's own render.
func render(entries map[int]Entry) []byte {
	ids := make([]int, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	var b strings.Builder
	b.WriteString(`// Parallax Mod Manager - a partial cache of LoversLab file details this app
// has separately learned by actually opening that file's own detail view
// (its author's real avatar, view count, and real updated date), since the
// Browse tab's own browsing-grid listing page never shows any of these three
// at all. Only ever grows as a side effect of a real visit, never fetched
// on purpose to fill this in - a file not listed here just hasn't been
// opened yet. Deleting an entry (or this whole file) loses nothing but that
// head start; it fills back in the next time that file's own page is opened.
{
  "files": {
`)
	for i, id := range ids {
		e := entries[id]
		b.WriteString("    " + quote(strconv.Itoa(id)) + ": {" +
			"\"authorAvatarUrl\": " + quote(e.AuthorAvatarURL) + ", " +
			"\"views\": " + fmt.Sprint(e.Views) + ", " +
			"\"updated\": " + quote(e.Updated) + ", " +
			"\"cachedAt\": " + fmt.Sprint(e.CachedAt) +
			"}")
		if i < len(ids)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }\n}\n")
	return []byte(b.String())
}
