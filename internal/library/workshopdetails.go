package library

import (
	"context"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// WorkshopDetailsCache holds real Steam Workshop metadata (title,
// description, subscriber/view counts, last-updated time), fetched once
// per published file id and kept in memory for the app's runtime - see
// docs/steam-web-api.md. The zero value is ready to use.
type WorkshopDetailsCache struct {
	mu   sync.Mutex
	byID map[string]steamapi.PublishedFileDetails
	// fetch defaults to steamapi.GetPublishedFileDetails (see Get) - a
	// field rather than a direct call so tests can substitute a fake that
	// never makes a real network request.
	fetch func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error)
}

// Get returns real Steam Workshop metadata for every one of cfg's
// currently scanned Workshop mods. An id already fetched in a previous
// call (this process, this session) is served from memory with no network
// call at all; only ids genuinely missing get batched into a single
// request - see steamapi.GetPublishedFileDetails. A mod Steam doesn't
// recognize, or that this cache hasn't been asked about yet, is simply
// absent from the result.
func (c *WorkshopDetailsCache) Get(ctx context.Context, cfg game.GameConfig, opts Options) (map[string]steamapi.PublishedFileDetails, error) {
	return c.get(ctx, cfg, opts, false)
}

// GetFresh is Get that asks Steam again for every one of cfg's Workshop mods,
// replacing what was remembered - for a "check again" that has to see an
// update published since the details were first fetched this session.
func (c *WorkshopDetailsCache) GetFresh(ctx context.Context, cfg game.GameConfig, opts Options) (map[string]steamapi.PublishedFileDetails, error) {
	return c.get(ctx, cfg, opts, true)
}

func (c *WorkshopDetailsCache) get(ctx context.Context, cfg game.GameConfig, opts Options, refetchAll bool) (map[string]steamapi.PublishedFileDetails, error) {
	scanResult, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: opts.SteamRoots, ModDir: opts.ModDir, ExtraFolders: opts.ExtraFolders})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(scanResult.Mods))
	for _, m := range scanResult.Mods {
		if m.Source == mod.SourceWorkshop && m.Descriptor.RemoteFileID != "" {
			ids = append(ids, m.Descriptor.RemoteFileID)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byID == nil {
		c.byID = map[string]steamapi.PublishedFileDetails{}
	}

	missing := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := c.byID[id]; !ok || refetchAll {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		fetch := c.fetch
		if fetch == nil {
			fetch = steamapi.GetPublishedFileDetails
		}
		fetched, err := fetch(ctx, missing)
		if err != nil {
			return nil, err
		}
		for id, d := range fetched {
			c.byID[id] = d
		}
	}

	result := make(map[string]steamapi.PublishedFileDetails, len(ids))
	for _, id := range ids {
		if d, ok := c.byID[id]; ok {
			result[id] = d
		}
	}
	return result, nil
}
