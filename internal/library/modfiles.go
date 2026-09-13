package library

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// maxModFileEntries caps how many entries ListModFiles returns - some
// total-conversion mods ship many thousands of files, and a UI file tree
// has no use rendering all of them. TotalSize still reflects every real
// file regardless of the cap.
const maxModFileEntries = 2000

// FileEntry is one file or folder inside a mod's real content directory.
type FileEntry struct {
	RelPath string // forward-slashed, relative to the mod's content root
	IsDir   bool
	Size    int64 // 0 for directories
}

// ModFiles is one mod's real on-disk content, for a UI file-tree view.
type ModFiles struct {
	Entries []FileEntry
	// TotalSize sums every real file under the mod's content root, even
	// past Entries' cap.
	TotalSize int64
	// Truncated is true when there were more entries than
	// maxModFileEntries - Entries is a prefix, not the full listing.
	Truncated bool
	// LastModified is the newest mtime among every real file under the
	// mod's content root, Unix seconds (0 if the directory has no files or
	// every stat failed) - safe to cross the Wails/JS boundary as a plain
	// number, unlike a raw hash (see CLAUDE.md's boundary note): a Unix
	// second count is nowhere near JS's 2^53 safe-integer limit.
	LastModified int64
}

// ListModFiles finds modID among cfg's scanned mods and lists every file
// and folder under its real content directory - see maxModFileEntries for
// the cap on how many are actually returned.
func ListModFiles(ctx context.Context, cfg game.GameConfig, opts Options, modID string) (ModFiles, error) {
	m, err := findMod(ctx, cfg, opts, modID)
	if err != nil {
		return ModFiles{}, err
	}

	// Entries starts as a real empty slice, not nil - an empty-content mod
	// would otherwise cross the JS boundary as null where the frontend's
	// type says FileEntry[], and crash the first .length/.map call on it.
	result := ModFiles{Entries: []FileEntry{}}
	walkErr := filepath.WalkDir(m.ContentPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == m.ContentPath {
			return nil // the root itself isn't an entry
		}
		rel, relErr := filepath.Rel(m.ContentPath, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		entry := FileEntry{RelPath: rel, IsDir: d.IsDir()}
		if !d.IsDir() {
			if info, infoErr := d.Info(); infoErr == nil {
				entry.Size = info.Size()
				result.TotalSize += entry.Size
				if mtime := info.ModTime().Unix(); mtime > result.LastModified {
					result.LastModified = mtime
				}
			}
		}

		if len(result.Entries) < maxModFileEntries {
			result.Entries = append(result.Entries, entry)
		} else {
			result.Truncated = true
		}
		return nil
	})
	if walkErr != nil {
		return ModFiles{}, fmt.Errorf("library: listing files for %s: %w", modID, walkErr)
	}

	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].RelPath < result.Entries[j].RelPath })
	return result, nil
}

// ModFolderPath finds modID among cfg's scanned mods and returns its real
// content directory - the caller (app.go) is responsible for actually
// opening it in the OS file manager, per this project's "real OS side
// effects live in the composition layer" rule.
func ModFolderPath(ctx context.Context, cfg game.GameConfig, opts Options, modID string) (string, error) {
	m, err := findMod(ctx, cfg, opts, modID)
	if err != nil {
		return "", err
	}
	return m.ContentPath, nil
}

// ModSizes returns every one of cfg's scanned mods' real total content
// size, computed in parallel (bounded to runtime.NumCPU()) since a
// cross-game library view can mean walking dozens of mods' content folders
// at once. A mod whose content can't be walked (rare - a stale or removed
// path) is simply left out of the result rather than failing the batch,
// matching this project's non-fatal-per-item philosophy elsewhere.
func ModSizes(ctx context.Context, cfg game.GameConfig, opts Options) (map[string]int64, error) {
	scanResult, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: opts.SteamRoots, ModDir: opts.ModDir})
	if err != nil {
		return nil, fmt.Errorf("library: scanning %s: %w", cfg.ID, err)
	}

	sizes := make(map[string]int64, len(scanResult.Mods))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())
	for _, m := range scanResult.Mods {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			total, err := dirSize(m.ContentPath)
			if err != nil {
				return nil
			}
			mu.Lock()
			sizes[m.ID] = total
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return sizes, nil
}

// dirSize sums every real file's size under root - no per-entry list kept,
// unlike ListModFiles, since a library-wide size pass has no use for one.
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// maxReadModFileSize caps ReadModFile's result - real mod content files are
// small Clausewitz/localisation text; this exists to compare exactly that,
// not to serve arbitrary large or binary assets through the UI.
const maxReadModFileSize = 512 * 1024

// ReadModFile returns one real file's text content from inside modID's
// content directory, for the conflict resolver's side-by-side view.
// relPath must be a mod-relative path (as ConflictCandidate.FilePath
// already is) - resolved and verified to stay inside the mod's own content
// directory, so a malformed or unexpected relPath can never read anything
// outside it.
func ReadModFile(ctx context.Context, cfg game.GameConfig, opts Options, modID, relPath string) (string, error) {
	m, err := findMod(ctx, cfg, opts, modID)
	if err != nil {
		return "", err
	}

	root, err := filepath.Abs(m.ContentPath)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(filepath.Join(m.ContentPath, relPath))
	if err != nil {
		return "", err
	}
	if absPath != root && !strings.HasPrefix(absPath, root+string(filepath.Separator)) {
		return "", fmt.Errorf("library: %q is outside %s's content directory", relPath, modID)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("library: %q is a directory, not a file", relPath)
	}
	if info.Size() > maxReadModFileSize {
		return "", fmt.Errorf("library: %s is too large to preview (%d bytes)", relPath, info.Size())
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// findMod re-scans cfg (the same discovery LoadGame itself uses) and
// returns the one mod matching modID.
func findMod(ctx context.Context, cfg game.GameConfig, opts Options, modID string) (mod.Mod, error) {
	scanResult, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: opts.SteamRoots, ModDir: opts.ModDir})
	if err != nil {
		return mod.Mod{}, fmt.Errorf("library: scanning %s: %w", cfg.ID, err)
	}
	for _, m := range scanResult.Mods {
		if m.ID == modID {
			return m, nil
		}
	}
	return mod.Mod{}, fmt.Errorf("library: mod %q not found in %s", modID, cfg.ID)
}
