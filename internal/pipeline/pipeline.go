// Package pipeline wires the cache and parsers together with a bounded
// worker pool: parses one mod's content files in parallel, merges the
// results in deterministic order, and reports per-mod progress. See
// docs/performance-strategy.md points 3 and 6.
package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
	"github.com/Official-Husko/parallax-mod-manager/internal/xhash"
)

// FileResult is one file's outcome from a LoadMod run.
type FileResult struct {
	RelPath     string
	Record      cache.FileRecord
	Definitions []definition.Definition
	Err         error
}

// Progress reports how far a LoadMod run has gotten, for per-mod (not
// per-phase) progress reporting - see docs/performance-strategy.md point 6.
type Progress struct {
	ModID      string
	FilesDone  int
	FilesTotal int
}

// Options configures a LoadMod run.
type Options struct {
	Store Store
	// Workers caps how many files are parsed concurrently. 0 (the zero
	// value) means runtime.NumCPU() - parsing is CPU-bound, so
	// oversubscribing beyond that adds scheduling overhead without benefit.
	Workers int
	// OnProgress, if set, is called after every file finishes (from
	// whichever goroutine finished it - it must be safe to call
	// concurrently, or do its own synchronization).
	OnProgress func(Progress)
}

// Store is the subset of cache.Store this package needs - defined locally
// so tests can supply a lightweight fake without depending on cache's own
// on-disk FileStore.
type Store interface {
	Load(ctx context.Context, gameKey, modID string) (*cache.ModCache, error)
	Save(ctx context.Context, c *cache.ModCache) error
}

// LoadMod parses every content file of one mod (that lives under one of
// cfg.ScanFolders), reusing opts.Store's cached results for anything whose
// (mtime,size) - or, failing that, content hash - hasn't changed since the
// last run.
//
// Parsing runs in a worker pool sized to opts.Workers (default
// runtime.NumCPU()), one goroutine per file, each writing only to its own
// fixed index of a preallocated slice - ordering falls out of array
// indexing, with no locks and no channel-reordering step. Everything after
// the pool (populating the cache and building the merged Definitions slice)
// happens single-threaded, in file order, matching the parallel-parse /
// sequential-merge split docs/performance-strategy.md calls for: mod
// load-order semantics depend on processing files/mods in a stable order, so
// that step must never run concurrently.
func LoadMod(ctx context.Context, m mod.Mod, cfg game.GameConfig, opts Options) ([]definition.Definition, error) {
	files, err := enumerateFiles(m.ContentPath, cfg.ScanFolders)
	if err != nil {
		return nil, err
	}

	modCache, err := opts.Store.Load(ctx, cfg.Key, m.ID)
	if err != nil {
		return nil, err
	}

	results := make([]FileResult, len(files))

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	var done int32
	for i, rel := range files {
		i, rel := i, rel
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			results[i] = processFile(modCache, m.ID, m.ContentPath, rel)
			if opts.OnProgress != nil {
				opts.OnProgress(Progress{
					ModID:      m.ID,
					FilesDone:  int(atomic.AddInt32(&done, 1)),
					FilesTotal: len(files),
				})
			}
			return nil // per-file errors live in FileResult.Err; one bad file must not abort the whole mod
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Sequential merge: single-threaded, in file order. The only step that
	// mutates modCache or builds the merged slice.
	var defs []definition.Definition
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		modCache.Put(r.RelPath, r.Record)
		defs = append(defs, r.Definitions...)
	}

	if err := opts.Store.Save(ctx, modCache); err != nil {
		return nil, err
	}
	return defs, nil
}

// processFile implements stat -> hash -> parse for one file: a (mtime,size)
// hit reuses the cached Definitions with zero I/O; a stat miss but matching
// content hash (the "touched but not modified" case) also reuses the cached
// Definitions, refreshing only the stat fields; anything else gets parsed
// fresh.
func processFile(modCache *cache.ModCache, modID, contentRoot, relPath string) FileResult {
	absPath := filepath.Join(contentRoot, relPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return FileResult{RelPath: relPath, Err: err}
	}

	if rec, ok := modCache.Lookup(relPath, info); ok {
		return FileResult{RelPath: relPath, Record: rec, Definitions: rec.Definitions}
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return FileResult{RelPath: relPath, Err: err}
	}
	hash := xhash.Bytes(data)

	if prev, ok := modCache.Files[relPath]; ok && prev.Hash == hash {
		rec := prev
		rec.ModTimeUnixNano = info.ModTime().UnixNano()
		rec.Size = info.Size()
		return FileResult{RelPath: relPath, Record: rec, Definitions: rec.Definitions}
	}

	defs, parseErr := parseFile(modID, relPath, data)
	rec := cache.FileRecord{
		Path:            relPath,
		ModTimeUnixNano: info.ModTime().UnixNano(),
		Size:            info.Size(),
		Hash:            hash,
		Definitions:     defs,
	}
	if parseErr != nil {
		rec.ParseError = parseErr.Error()
	}
	return FileResult{RelPath: relPath, Record: rec, Definitions: defs}
}

// parseFile dispatches on file extension: .yml is localization, everything
// else under a scanned content folder is Clausewitz script.
func parseFile(modID, relPath string, data []byte) ([]definition.Definition, error) {
	defType := definition.Type(filepath.ToSlash(filepath.Dir(relPath)))

	if filepath.Ext(relPath) == ".yml" {
		cat, err := locale.Parse(data)
		if err != nil {
			return nil, err
		}
		return definition.FromLocaleCatalog(modID, relPath, cat), nil
	}

	f, err := script.Parse(data)
	if err != nil {
		return nil, err
	}
	return definition.FromScriptFile(modID, relPath, defType, f), nil
}
