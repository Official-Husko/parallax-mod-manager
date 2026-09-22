// Package backup keeps 1:1 copies of Steam Workshop mods, so a mod that is deleted
// from the Workshop (or made private) is not lost when Steam removes its files.
//
// Steam cannot be asked to hold a deletion back, and nothing outside it can see one
// coming, so the only reliable moment to copy a mod is before Steam removes it: as
// soon as the app learns a mod is deleted or private while its files are still on
// disk (see internal/steamapi.Classify), or, in the "every mod" mode, ahead of any
// trouble.
//
// Layout, under a root folder the user can choose (default: a Parallax Mod Backups
// folder inside the app's settings folder):
//
//	<root>/<game id>/mods/<workshop item id>/...   the mod's folder, copied as it is
//	<root>/<game id>/backups.jsonc                 what was copied, when and why
//
// The mod folders hold nothing but the mod's own files, so one can be dropped back
// into the Workshop or mod folder as it is.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/fsutil"
)

// itemIDPattern is what a Workshop item id looks like: it names the folder a mod's
// copy is kept in, the same name Steam gives the folder the mod lives in.
var itemIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

// Source is one mod to copy.
type Source struct {
	// GameID names the game's folder (its permanent id).
	GameID string
	// ModID is the app's id for the mod (for showing).
	ModID string
	// RemoteFileID is the Workshop item id, which names the copy's folder. Digits only:
	// it comes from a mod's own descriptor, so it must never be able to point outside
	// the backup folder.
	RemoteFileID string
	Name         string
	Version      string
	// ContentPath is the folder to copy.
	ContentPath string
	// Reason is why it is being copied: ReasonDeleted, ReasonPrivate, ReasonAll or
	// ReasonManual.
	Reason string
}

// Why a copy was made (Entry.Reason).
const (
	ReasonDeleted = "deleted"
	ReasonPrivate = "private"
	ReasonAll     = "all"
	ReasonManual  = "manual"
)

// Result is what Copy did.
type Result struct {
	Entry Entry
	// Skipped is true when an up-to-date copy was already there, so nothing was written.
	Skipped bool
}

// Progress reports how far a copy has got, in bytes. It is called once with 0 done
// just before any file is copied (so a caller learns a copy has really started, not
// been skipped as up to date), then after each file.
type Progress func(doneBytes, totalBytes int64)

// Errors Copy returns that a caller can tell apart.
var (
	// ErrNoSpace means the volume the backups live on does not have room for the mod.
	ErrNoSpace = errors.New("backup: not enough free space in the backup folder")
	// ErrSourceGone means the mod's folder is not there to copy any more.
	ErrSourceGone = errors.New("backup: the mod's files are no longer on disk")
	// ErrOverLimit means the copy would take the backups past the size cap the user set.
	ErrOverLimit = errors.New("backup: the backup size limit would be exceeded")
	// ErrLowSpace means the copy would leave the drive with less free space than the user
	// asked to be kept.
	ErrLowSpace = errors.New("backup: too little free space would be left")
	// ErrIncomplete means files vanished while copying (Steam removing the mod under
	// us) and an older complete copy was kept instead of a partial one.
	ErrIncomplete = errors.New("backup: files were removed while copying")
)

// freeBytes is diskFree; a variable so tests can pretend the disk is full.
var freeBytes = diskFree

// DiskFree is diskFree, exported for callers outside this package that need a target folder's
// free space (see App.PreviewDuplicateMod) without reimplementing the platform-specific check
// this package already has.
func DiskFree(path string) (uint64, bool) { return diskFree(path) }

// spaceMargin is left free on the volume on top of the mod's own size.
const spaceMargin = 64 << 20

// Limits are the two user-set rules a copy is held to. A zero value is no rule.
type Limits struct {
	// MaxTotal is the most every backup together may take, in bytes: a copy that would
	// go past it is refused. The size a mod's older copy already takes does not count
	// twice when it is replaced.
	MaxTotal int64
	// MinFree is the free space that must be left on the volume after the copy.
	MinFree int64
}

// Copy is CopyWithLimits with no limits.
func Copy(ctx context.Context, root string, src Source, progress Progress) (Result, error) {
	return CopyWithLimits(ctx, root, src, progress, Limits{})
}

// CopyWithLimits makes (or refreshes) the copy of src under root. It is safe to call again for
// a mod that is already backed up: an unchanged mod is skipped, a changed one is
// copied anew and only then replaces the old copy.
//
// The copy is made into a temporary folder next to its place and moved into place
// when finished, so a crash or a cancel never leaves a half-written copy where a
// good one was. Files that vanish while copying (Steam removing the mod as it is
// copied) are counted: if that leaves a copy with files missing it is kept only when
// there is no complete one already, and is marked incomplete.
func CopyWithLimits(ctx context.Context, root string, src Source, progress Progress, limits Limits) (Result, error) {
	if root == "" {
		return Result{}, errors.New("backup: no backup folder is set")
	}
	if !itemIDPattern.MatchString(src.RemoteFileID) {
		return Result{}, fmt.Errorf("backup: %q is not a Workshop item id", src.RemoteFileID)
	}
	if src.GameID == "" || strings.ContainsAny(src.GameID, `/\`) || src.GameID == "." || src.GameID == ".." {
		return Result{}, fmt.Errorf("backup: %q is not a game id", src.GameID)
	}
	info, err := os.Stat(src.ContentPath)
	if err != nil || !info.IsDir() {
		return Result{}, ErrSourceGone
	}

	gameDir := filepath.Join(root, src.GameID)
	dest := filepath.Join(gameDir, "mods", src.RemoteFileID)
	if err := refuseOverlap(src.ContentPath, dest); err != nil {
		return Result{}, err
	}

	fp := fingerprint(src.ContentPath)
	idx := loadIndex(gameDir)
	if old, ok := idx.Mods[src.RemoteFileID]; ok && old.Complete && old.Files == fp.files && old.Size == fp.size && old.Newest == fp.newest {
		if _, err := os.Stat(dest); err == nil {
			return Result{Entry: old, Skipped: true}, nil
		}
	}

	if err := os.MkdirAll(filepath.Join(gameDir, "mods"), 0o755); err != nil {
		return Result{}, fmt.Errorf("backup: creating %s: %w", gameDir, err)
	}
	if limits.MaxTotal > 0 {
		used := TotalSize(root)
		if old, ok := idx.Mods[src.RemoteFileID]; ok {
			used -= old.Size // replaced, not added to
			if used < 0 {
				used = 0
			}
		}
		if used+fp.size > limits.MaxTotal {
			return Result{}, fmt.Errorf("%w (%s used of %s, this mod needs %s)", ErrOverLimit, humanBytes(used), humanBytes(limits.MaxTotal), humanBytes(fp.size))
		}
	}
	if free, ok := freeBytes(gameDir); ok {
		if free < uint64(fp.size)+spaceMargin {
			return Result{}, fmt.Errorf("%w (needs %s, %s free)", ErrNoSpace, humanBytes(fp.size), humanBytes(int64(free)))
		}
		if limits.MinFree > 0 && free < uint64(fp.size)+uint64(limits.MinFree) {
			return Result{}, fmt.Errorf("%w (%s would be left of the %s you keep free; %s free now, this mod needs %s)",
				ErrLowSpace, humanBytes(max(int64(free)-fp.size, 0)), humanBytes(limits.MinFree), humanBytes(int64(free)), humanBytes(fp.size))
		}
	}

	partial := dest + ".partial"
	if err := os.RemoveAll(partial); err != nil {
		return Result{}, fmt.Errorf("backup: clearing %s: %w", partial, err)
	}
	var fsProgress fsutil.Progress
	if progress != nil {
		fsProgress = func(_ string, done, total int64) { progress(done, total) }
	}
	stats, err := fsutil.CopyTree(ctx, src.ContentPath, partial, fp.size, fsProgress)
	if err != nil {
		_ = os.RemoveAll(partial)
		return Result{}, err
	}

	// Files that vanished between counting the source and walking it were never seen
	// by the walk, so count them here.
	if stats.Files+stats.Missing < fp.files {
		stats.Missing = fp.files - stats.Files
	}
	incomplete := stats.Missing > 0
	if incomplete {
		if old, ok := idx.Mods[src.RemoteFileID]; ok && old.Complete {
			_ = os.RemoveAll(partial)
			return Result{}, fmt.Errorf("%w (%d of %d files); the earlier copy was kept", ErrIncomplete, stats.Missing, stats.Files+stats.Missing)
		}
	}

	// Move the new copy into place; the old one goes only once the new is there.
	old := dest + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, old); err != nil {
			_ = os.RemoveAll(partial)
			return Result{}, fmt.Errorf("backup: replacing the earlier copy: %w", err)
		}
	}
	if err := os.Rename(partial, dest); err != nil {
		_ = os.Rename(old, dest) // put the earlier copy back
		_ = os.RemoveAll(partial)
		return Result{}, fmt.Errorf("backup: moving the copy into place: %w", err)
	}
	_ = os.RemoveAll(old)

	entry := Entry{
		ModID:        src.ModID,
		RemoteFileID: src.RemoteFileID,
		Name:         src.Name,
		Version:      src.Version,
		BackedUpAt:   time.Now().Unix(),
		Reason:       src.Reason,
		Files:        stats.Files,
		Size:         stats.Bytes,
		Newest:       fp.newest,
		Complete:     !incomplete,
		Missing:      stats.Missing,
	}
	// An incomplete copy never matches the source's fingerprint, so the next check
	// tries again; a complete one records the source's, to skip until it changes.
	if !incomplete {
		entry.Files, entry.Size = fp.files, fp.size
	}
	if err := recordEntry(gameDir, entry); err != nil {
		return Result{Entry: entry}, fmt.Errorf("backup: the copy was made but could not be recorded: %w", err)
	}
	return Result{Entry: entry}, nil
}

// refuseOverlap stops a copy that would write into the mod it is copying, or copy a
// backup folder into itself.
func refuseOverlap(src, dest string) error {
	a, b := resolve(src), resolve(dest)
	if a == b || within(a, b) || within(b, a) {
		return fmt.Errorf("backup: the backup folder overlaps the mod's own folder (%s); choose a folder outside the mods", src)
	}
	return nil
}

// resolve is a clean absolute path with symlinks followed as far as they exist.
func resolve(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	// Follow links on the longest existing prefix, so a not-yet-created destination
	// still resolves through the links above it.
	rest := ""
	cur := abs
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// within says whether child is inside parent (and not the same folder).
func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

type fingerprintResult struct {
	files  int
	size   int64
	newest int64
}

// fingerprint counts the files under root, their total size and the newest
// modification time - the same summary update tracking uses to tell a mod changed.
func fingerprint(root string) fingerprintResult {
	var f fingerprintResult
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		f.files++
		f.size += info.Size()
		if t := info.ModTime().Unix(); t > f.newest {
			f.newest = t
		}
		return nil
	})
	return f
}

// humanBytes formats a size for messages.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
