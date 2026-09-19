package library

import (
	"os"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// ContentPathCache remembers where each of a game's mods keeps its real
// content, as of the last full scan, so a caller that only needs one mod's
// folder (ReadModFile - one call per side every time the Conflict Resolver
// switches to another contested key) doesn't have to re-run scan.Scan to
// find it. A scan reads every descriptor, walks the Workshop directory, and
// recursively walks every extra mod folder, which on a big modlist (or one
// on a network drive) is seconds of work spent to answer a question the
// scan that fed the Conflict Resolver its list already answered. The zero
// value is ready to use, and every method is safe for concurrent use.
type ContentPathCache struct {
	mu     sync.Mutex
	byGame map[string]map[string]string // game ID -> mod ID -> content directory
}

// Store replaces gameID's remembered paths with mods' - a mod that's gone
// from mods is forgotten too. A mod whose content is missing has no
// directory worth remembering, so it's left out (findMod's own error for
// it, explaining why, is what a caller should see). Safe to call on a nil
// receiver, so Options.ContentPaths stays optional.
func (c *ContentPathCache) Store(gameID string, mods []mod.Mod) {
	if c == nil {
		return
	}
	paths := make(map[string]string, len(mods))
	for _, m := range mods {
		if m.ContentMissing || m.ContentPath == "" {
			continue
		}
		paths[m.ID] = m.ContentPath
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byGame == nil {
		c.byGame = map[string]map[string]string{}
	}
	c.byGame[gameID] = paths
}

// lookup returns modID's remembered content directory, but only if that
// directory still exists on disk - a stale entry (the mod was moved or
// deleted since the last scan) reports a miss, so the caller falls back to
// a fresh scan rather than trusting a path that no longer leads anywhere.
func (c *ContentPathCache) lookup(gameID, modID string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	dir, ok := c.byGame[gameID][modID]
	c.mu.Unlock()
	if !ok {
		return "", false
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", false
	}
	return dir, true
}

// remember records one mod's content directory without touching the rest of
// gameID's entries - used after a cache miss forced a real scan for just
// this mod.
func (c *ContentPathCache) remember(gameID, modID, dir string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byGame == nil {
		c.byGame = map[string]map[string]string{}
	}
	if c.byGame[gameID] == nil {
		c.byGame[gameID] = map[string]string{}
	}
	c.byGame[gameID][modID] = dir
}
