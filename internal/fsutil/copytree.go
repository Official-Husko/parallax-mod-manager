// Package fsutil holds generic filesystem primitives shared by more than one feature - copying a
// whole directory tree, and totalling one up - kept separate from any one feature's own package
// (internal/backup, internal/modedit) so neither has to import the other just to reuse this.
package fsutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Progress reports the file currently being copied (its path relative to the tree's own root)
// and how far the whole copy has got, in bytes. It is called once with ("", 0, totalBytes) just
// before any file is copied (so a caller learns a copy has really started), then again after
// each file.
type Progress func(file string, doneBytes, totalBytes int64)

// Stats is what CopyTree copied.
type Stats struct {
	Files int
	Bytes int64
	// Missing counts files that vanished while copying - not fatal on their own.
	Missing int
	// Skipped counts symlinks and other non-regular files, which are never copied.
	Skipped int
}

// copyBufferSize is the read size for file copies: large, since mods hold big textures and sound
// files.
const copyBufferSize = 1 << 20

// CopyTree copies the regular files and folders under src into dst, keeping each file's
// permissions and modification time. A file that disappears part-way is counted as missing, not
// fatal. totalBytes is only used to report progress (see Progress) - pass the result of a prior
// DirStat call, or 0 if it is not known. Cancelling ctx stops the copy promptly, mid-file.
func CopyTree(ctx context.Context, src, dst string, totalBytes int64, progress Progress) (Stats, error) {
	return copyTree(ctx, src, dst, totalBytes, progress, nil)
}

// CopyTreeExcluding is CopyTree, skipping every path in exclude - each a
// forward-slashed path relative to src, matching library.FileEntry.RelPath's
// own convention (not imported here - see that package for why - but the
// same shape). Excluding a folder skips everything under it too, without
// this needing to walk that subtree at all. Built for
// internal/workshop's own "leave these files out of the Workshop upload"
// feature, but kept here rather than in that package since skipping some
// paths while copying a tree is a generic filesystem operation, not a
// Workshop-specific one.
func CopyTreeExcluding(ctx context.Context, src, dst string, exclude map[string]bool, totalBytes int64, progress Progress) (Stats, error) {
	return copyTree(ctx, src, dst, totalBytes, progress, exclude)
}

// isExcluded reports whether rel itself, or any of its ancestor directories,
// is in exclude.
func isExcluded(rel string, exclude map[string]bool) bool {
	if exclude[rel] {
		return true
	}
	for {
		parent := filepath.ToSlash(filepath.Dir(rel))
		if parent == "." || parent == rel {
			return false
		}
		if exclude[parent] {
			return true
		}
		rel = parent
	}
}

func copyTree(ctx context.Context, src, dst string, totalBytes int64, progress Progress, exclude map[string]bool) (Stats, error) {
	var stats Stats
	buf := make([]byte, copyBufferSize)
	var done int64
	if progress != nil {
		progress("", 0, totalBytes)
	}
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				stats.Missing++
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		// rel is "." for the root itself (path == src) - never excluded (no
		// real relative path is ever "."), so the root's own os.MkdirAll
		// below still runs even for an empty src, exactly as CopyTree always
		// has.
		if len(exclude) > 0 && isExcluded(rel, exclude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case !d.Type().IsRegular():
			stats.Skipped++
			return nil
		}
		n, err := copyFile(ctx, path, target, buf)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			stats.Missing++
			_ = os.Remove(target)
			return nil
		case err != nil:
			return err
		}
		stats.Files++
		stats.Bytes += n
		done += n
		if progress != nil {
			progress(rel, done, totalBytes)
		}
		return nil
	})
	if walkErr != nil {
		return stats, walkErr
	}
	return stats, nil
}

// copyFile copies one file, checks the size arrived intact and restores its time and
// permissions. A source that is gone (or vanishes while being read) is ErrNotExist.
func copyFile(ctx context.Context, from, to string, buf []byte) (int64, error) {
	in, err := os.Open(from)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return 0, err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm()|0o200)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.CopyBuffer(out, &ctxReader{ctx: ctx, r: in}, buf)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(to)
		return 0, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(to)
		return 0, closeErr
	}
	if n != info.Size() {
		// The file changed size under us (a download still writing to it, or Steam
		// removing it): it is not a faithful copy.
		_ = os.Remove(to)
		return 0, fmt.Errorf("%s changed while it was being copied: %w", from, fs.ErrNotExist)
	}
	_ = os.Chmod(to, info.Mode().Perm())
	_ = os.Chtimes(to, time.Now(), info.ModTime())
	return n, nil
}

// ctxReader makes a long copy stop promptly when its context is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// DirStat walks path and totals its regular files - what CopyTree's totalBytes argument needs,
// and what a size warning before a big copy shows.
func DirStat(path string) (files int, bytes int64, err error) {
	err = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes, err
}
