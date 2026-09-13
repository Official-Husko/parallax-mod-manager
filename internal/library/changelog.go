package library

import (
	"context"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// ChangelogCache holds real Steam Workshop update notes in memory for the
// app's runtime, keyed by published file id - see
// steamapi.GetChangelog. Unlike WorkshopDetailsCache, this is never
// batched across every scanned mod: there's no batching endpoint for it
// (it's a real HTML page fetch per mod, see docs/steam-web-api.md), so a
// mod's changelog is only ever fetched the first time its own Changes tab
// is actually opened. The zero value is ready to use.
type ChangelogCache struct {
	mu   sync.Mutex
	byID map[string][]steamapi.ChangelogEntry
	// fetch defaults to steamapi.GetChangelog - a field rather than a
	// direct call so tests can substitute a fake that never makes a real
	// network request.
	fetch func(ctx context.Context, publishedFileID string) ([]steamapi.ChangelogEntry, error)
}

// Get returns publishedFileID's real Workshop update notes, fetching them
// only the first time this id is asked for and serving every later call
// from memory.
func (c *ChangelogCache) Get(ctx context.Context, publishedFileID string) ([]steamapi.ChangelogEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byID == nil {
		c.byID = map[string][]steamapi.ChangelogEntry{}
	}
	if entries, ok := c.byID[publishedFileID]; ok {
		return entries, nil
	}

	fetch := c.fetch
	if fetch == nil {
		fetch = steamapi.GetChangelog
	}
	entries, err := fetch(ctx, publishedFileID)
	if err != nil {
		return nil, err
	}
	c.byID[publishedFileID] = entries
	return entries, nil
}
