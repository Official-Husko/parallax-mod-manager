// Package loverslabcategories is a small, on-disk cache of the Browse tab's own sidebar - the
// real LoversLab "Paradox Games" category and its own subcategories (see
// internal/app/loverslab.go's LoversLabCategories). Building that sidebar costs two real page
// fetches that must run one after another (the second page's own URL is only known once the
// first response has been parsed), confirmed live to cost over a second combined on a cold
// fetch.
//
// internal/loverslab's own Client already caches a fetched page for a while (client.go's
// getDocument/pageCacheTTL, three minutes) - but that cache lives only as long as the *Client*
// does, and is gone the moment a fresh one replaces it (a session re-verification, a restart).
// The sidebar's own real structure changes on a timescale of essentially never (a new Paradox
// game getting its own LoversLab subcategory is a rare, noticed event), so it is worth caching
// far longer than three minutes and across restarts - see StaleAfter - rather than paying that
// second-plus cost on every single Browse visit that happens to be the first one of a run.
package loverslabcategories

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
)

// StaleAfter is how long a cached sidebar is trusted before the next Browse visit pays for a
// real refetch again - long enough that a normal session, even one left open for hours, never
// notices, short enough that a real (if rare) change to the site's own category structure is
// still picked up within a day rather than needing someone to notice and delete this file by
// hand.
const StaleAfter = 24 * time.Hour

// Store keeps the cached sidebar on disk as one file in Dir.
type Store struct {
	Dir string
}

func (s Store) path() string {
	return filepath.Join(s.Dir, "browse_categories.jsonc")
}

// cachedCategory mirrors loverslab.Category's fields under this file's own on-disk names, kept
// separate so this cache's shape doesn't depend on (or constrain) that type's own field names.
type cachedCategory struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Files int    `json:"files"`
	Depth int    `json:"depth"`
}

type cachedFile struct {
	Categories []cachedCategory `json:"categories"`
	CachedAt   int64            `json:"cachedAt"`
}

// Load returns the cached sidebar and whether it's still fresh (within StaleAfter). A missing,
// unreadable or corrupt cache reports ok=false with a nil slice and no error - a cache miss is
// the normal, expected first-run state, never a reason to fail Browse over; the caller is
// expected to fetch fresh and Save the result. fresh is only meaningful when len(categories) > 0.
func (s Store) Load() (categories []loverslab.Category, fresh bool, err error) {
	if s.Dir == "" {
		return nil, false, nil
	}
	data, readErr := os.ReadFile(s.path())
	if errors.Is(readErr, os.ErrNotExist) {
		return nil, false, nil
	}
	if readErr != nil {
		return nil, false, fmt.Errorf("loverslabcategories: reading %s: %w", s.path(), readErr)
	}
	var f cachedFile
	if err := jsonc.Unmarshal(data, &f); err != nil {
		// Corrupt cache: treated the same as no cache at all, not a fatal error - it is
		// simply rebuilt on the next real fetch.
		return nil, false, nil
	}
	categories = make([]loverslab.Category, len(f.Categories))
	for i, c := range f.Categories {
		categories[i] = loverslab.Category{ID: c.ID, Name: c.Name, URL: c.URL, Files: c.Files, Depth: c.Depth}
	}
	fresh = len(categories) > 0 && time.Since(time.Unix(f.CachedAt, 0)) < StaleAfter
	return categories, fresh, nil
}

// Save writes categories atomically, replacing the file, timestamped as of now.
func (s Store) Save(categories []loverslab.Category) error {
	if s.Dir == "" {
		return errors.New("loverslabcategories: there is no settings folder to cache the sidebar in")
	}
	if _, err := atomicfile.Write(s.Dir, filepath.Base(s.path()), render(categories)); err != nil {
		return fmt.Errorf("loverslabcategories: saving: %w", err)
	}
	return nil
}

func render(categories []loverslab.Category) []byte {
	cached := make([]cachedCategory, len(categories))
	for i, c := range categories {
		cached[i] = cachedCategory{ID: c.ID, Name: c.Name, URL: c.URL, Files: c.Files, Depth: c.Depth}
	}
	body, err := json.MarshalIndent(cachedFile{Categories: cached, CachedAt: time.Now().Unix()}, "", "  ")
	if err != nil {
		// Every field here is a plain string/int slice - this cannot actually fail.
		panic("loverslabcategories: marshalling a plain category list: " + err.Error())
	}
	header := []byte(`// Parallax Mod Manager - a cache of the Browse tab's own sidebar (the real LoversLab
// "Paradox Games" category and its own subcategories), so opening Browse doesn't pay for two
// real page fetches on every single visit - this structure changes on a timescale of
// essentially never. Safe to delete: it is rebuilt, slowly, the next time Browse is opened.
`)
	return append(header, body...)
}
