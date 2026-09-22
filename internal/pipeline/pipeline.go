// Package pipeline wires the cache and parsers together with a bounded
// worker pool: parses one mod's content files in parallel, merges the
// results in deterministic order, and reports per-mod progress. See
// docs/performance-strategy.md points 3 and 6.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

// outcome is how a file's definitions were obtained.
type outcome int

const (
	outcomeCached  outcome = iota // (mtime,size) matched: reused with no I/O
	outcomeTouched                // stat changed but the content hash matched: reused, stat refreshed
	outcomeParsed                 // read and parsed fresh
)

// FileResult is one file's outcome from a LoadMod run.
type FileResult struct {
	RelPath     string
	Record      cache.FileRecord
	Definitions []definition.Definition
	Err         error
	outcome     outcome
}

// Stats counts what a run of LoadMod calls did, across as many mods as share
// one Stats - the numbers behind "how well is the cache working?". Safe for
// concurrent use.
type Stats struct {
	// Files is every file examined.
	Files atomic.Int64
	// Cached is how many were reused on their (mtime,size) alone.
	Cached atomic.Int64
	// Touched is how many had a new mtime or size but identical content.
	Touched atomic.Int64
	// Parsed is how many had to be read and parsed.
	Parsed atomic.Int64
	// ParseErrors is how many files currently have a parse problem: ones that
	// couldn't be read at all, and localisation files read with some lines
	// skipped. Counted on every run, not just the one that first hit them - a
	// problem is remembered in the cache, so a warm run reports the same files
	// rather than a misleading zero.
	ParseErrors atomic.Int64
	// Saved is how many mod caches were written back to disk (one that was
	// entirely unchanged is not written).
	Saved atomic.Int64

	mu       sync.Mutex
	problems []Problem
	dropped  int
}

// Problem is one file that couldn't be read cleanly, and why.
type Problem struct {
	ModID string
	// Path is relative to the mod's content folder, slash-separated.
	Path string
	Err  string
}

// maxProblems bounds how many Problems a Stats keeps: a mod that is broken
// throughout would otherwise turn one scan into thousands of log lines.
const maxProblems = 20

// note records ps, keeping the first maxProblems and only counting the rest.
func (s *Stats) note(ps []Problem) {
	if len(ps) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range ps {
		if len(s.problems) < maxProblems {
			s.problems = append(s.problems, p)
		} else {
			s.dropped++
		}
	}
}

// Problems returns the files noted as having a problem, up to a bound, and how
// many more there were beyond it.
//
// Only a run that itself hit a problem reports it: a file that failed to parse
// is remembered in the cache and not parsed again, so a warm run counts it in
// ParseErrors but doesn't repeat it here. That is what keeps the same broken
// file from being reported on every scan.
func (s *Stats) Problems() (problems []Problem, more int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Problem(nil), s.problems...), s.dropped
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
	// Stats, if set, is added to as files are handled - share one across
	// several LoadMod calls to total a whole scan.
	Stats *Stats
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
	files, err := EnumerateFiles(m.ContentPath, cfg.ScanFolders)
	if err != nil {
		return nil, err
	}

	modCache, err := opts.Store.Load(ctx, cfg.ID, m.ID)
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
	total := 0
	for _, r := range results {
		if r.Err == nil {
			total += len(r.Definitions)
		}
	}
	// Sized once: appending file by file grew this slice by doubling, copying
	// every definition (a hundred-odd bytes each) several times over.
	defs := make([]definition.Definition, 0, total)
	dirty := false
	var cached, touched, parsed, parseErrors int64
	var fresh []Problem
	for _, r := range results {
		if r.Err != nil {
			// A file that couldn't be read at all: it contributes nothing, and
			// there is no cached record to remember that by, so it is reported
			// every run.
			fresh = append(fresh, Problem{ModID: m.ID, Path: r.RelPath, Err: r.Err.Error()})
			continue
		}
		switch r.outcome {
		case outcomeCached:
			cached++
		case outcomeTouched:
			touched++
		case outcomeParsed:
			parsed++
		}
		if r.Record.ParseError != "" {
			parseErrors++
			if r.outcome == outcomeParsed {
				fresh = append(fresh, Problem{ModID: m.ID, Path: r.RelPath, Err: r.Record.ParseError})
			}
		}
		if prev, ok := modCache.Files[r.RelPath]; !ok || !sameRecord(prev, r.Record) {
			modCache.Put(r.RelPath, r.Record)
			dirty = true
		}
		defs = append(defs, r.Definitions...)
	}
	if opts.Stats != nil {
		opts.Stats.Files.Add(cached + touched + parsed)
		opts.Stats.Cached.Add(cached)
		opts.Stats.Touched.Add(touched)
		opts.Stats.Parsed.Add(parsed)
		opts.Stats.ParseErrors.Add(parseErrors)
		opts.Stats.note(fresh)
	}

	// Writing the cache back is the expensive half of a warm run (encoding
	// every definition of the mod), and on a warm run there is nothing new to
	// write - so only do it when a file was added, changed, or re-stat'd.
	if dirty {
		if err := opts.Store.Save(ctx, modCache); err != nil {
			return nil, err
		}
		if opts.Stats != nil {
			opts.Stats.Saved.Add(1)
		}
	}
	return defs, nil
}

// sameRecord reports whether two cached records for one file agree on
// everything that identifies its state. Definitions are deliberately not
// compared: they're a pure function of the content the Hash already covers.
func sameRecord(a, b cache.FileRecord) bool {
	return a.ModTimeUnixNano == b.ModTimeUnixNano && a.Size == b.Size && a.Hash == b.Hash && a.ParseError == b.ParseError
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
		return FileResult{RelPath: relPath, Record: rec, Definitions: rec.Definitions, outcome: outcomeCached}
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
		return FileResult{RelPath: relPath, Record: rec, Definitions: rec.Definitions, outcome: outcomeTouched}
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
	return FileResult{RelPath: relPath, Record: rec, Definitions: defs, outcome: outcomeParsed}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
		defs := definition.FromLocaleCatalog(modID, relPath, cat)
		if n := len(cat.Skipped); n > 0 {
			// The readable entries are kept and used; the note travels with the
			// record (FileRecord.ParseError) so the scan can say the file has a
			// problem, without losing everything else in it.
			return defs, fmt.Errorf("%d line%s skipped, first: %s", n, plural(n), cat.Skipped[0].Reason)
		}
		return defs, nil
	}

	f, err := script.Parse(data)
	if err != nil {
		return nil, err
	}
	return definition.FromScriptFile(modID, relPath, defType, f), nil
}
