package modedit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// KeptSaves is how many saves of one mod are remembered for Undo.
const KeptSaves = 10

// ErrChangedSince means a file was changed by something else after the last save, so restoring the
// old version would throw that change away.
var ErrChangedSince = errors.New("modedit: a file was changed since the last save, so it was not restored")

// ErrNothingToUndo means no saved version is left.
var ErrNothingToUndo = errors.New("modedit: there is no earlier save to go back to")

// Write is one file a save writes.
type Write struct {
	Path string
	Data []byte
}

// Store keeps the earlier versions of one mod's files, in a folder in the app's settings (never inside
// the mod, whose folder is what gets uploaded). Each save gets a folder of its own holding a copy of
// every file it replaced and a manifest saying what it wrote.
type Store struct {
	Dir string
	// Roots are the only folders a save may write in and an undo may restore into: the mod's own
	// folder and the game's mod folder.
	Roots []string
	// Now is time.Now unless a test says otherwise.
	Now func() time.Time
}

type manifestEntry struct {
	Path     string `json:"path"`
	Existed  bool   `json:"existed"`
	Backup   string `json:"backup"`
	AfterSHA string `json:"afterSha256"`
}

type manifest struct {
	SavedAt time.Time       `json:"savedAt"`
	Files   []manifestEntry `json:"files"`
}

const manifestHeader = `// Parallax Mod Manager - one save of a mod's files, kept so it can be undone.
//
// "files" lists every file the save wrote: its path, whether it existed before, the copy of its
// earlier contents kept next to this file ("backup"), and the SHA-256 of what was written
// ("afterSha256"). Undo restores the copies, but only for files that still hold what was written.
// The mod's own folder is never touched by this history.
`

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// within says whether path is below one of roots.
func within(path string, roots []string) bool {
	p := filepath.Clean(path)
	for _, r := range roots {
		r = filepath.Clean(r)
		if r == "" || r == "." {
			continue
		}
		if p == r || strings.HasPrefix(p, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Apply writes the files, first keeping a copy of each one it replaces. If a write fails, the files
// already written are put back and the save leaves no trace.
func (s Store) Apply(writes []Write) (time.Time, error) {
	if s.Dir == "" {
		return time.Time{}, errors.New("modedit: there is no settings folder to keep earlier versions in")
	}
	for _, w := range writes {
		if !within(w.Path, s.Roots) {
			return time.Time{}, fmt.Errorf("modedit: %s is outside the mod's folders", w.Path)
		}
	}
	at := s.now()
	dir := filepath.Join(s.Dir, at.Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return time.Time{}, fmt.Errorf("modedit: %w", err)
	}

	m := manifest{SavedAt: at}
	perms := make([]os.FileMode, len(writes))
	for i, w := range writes {
		entry := manifestEntry{Path: w.Path, AfterSHA: sha(w.Data)}
		perms[i] = 0o644
		if old, err := os.ReadFile(w.Path); err == nil {
			entry.Existed = true
			entry.Backup = strconv.Itoa(i) + ".bak"
			if err := os.WriteFile(filepath.Join(dir, entry.Backup), old, 0o644); err != nil {
				os.RemoveAll(dir)
				return time.Time{}, fmt.Errorf("modedit: keeping the earlier version of %s: %w", w.Path, err)
			}
			if info, err := os.Stat(w.Path); err == nil {
				perms[i] = info.Mode().Perm()
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			os.RemoveAll(dir)
			return time.Time{}, fmt.Errorf("modedit: reading %s: %w", w.Path, err)
		}
		m.Files = append(m.Files, entry)
	}
	body, _ := json.MarshalIndent(m, "", "  ")
	if _, err := atomicfile.Write(dir, "save.jsonc", append([]byte(manifestHeader), body...)); err != nil {
		os.RemoveAll(dir)
		return time.Time{}, err
	}

	for i, w := range writes {
		if err := writeKeepingPerm(w.Path, w.Data, perms[i]); err != nil {
			s.rollback(dir, m.Files[:i])
			os.RemoveAll(dir)
			return time.Time{}, fmt.Errorf("modedit: writing %s: %w", w.Path, err)
		}
	}
	s.prune()
	return at, nil
}

func writeKeepingPerm(path string, data []byte, perm os.FileMode) error {
	written, err := atomicfile.Write(filepath.Dir(path), filepath.Base(path), data)
	if err != nil {
		return err
	}
	return os.Chmod(written, perm)
}

// rollback puts back the files a failed save had already replaced.
func (s Store) rollback(dir string, done []manifestEntry) {
	for _, e := range done {
		restoreEntry(dir, e)
	}
}

func restoreEntry(dir string, e manifestEntry) error {
	if !e.Existed {
		err := os.Remove(e.Path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	old, err := os.ReadFile(filepath.Join(dir, e.Backup))
	if err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if info, err := os.Stat(e.Path); err == nil {
		perm = info.Mode().Perm()
	}
	return writeKeepingPerm(e.Path, old, perm)
}

// saves lists the save folders, oldest first.
func (s Store) saves() []string {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func (s Store) prune() {
	names := s.saves()
	for len(names) > KeptSaves {
		os.RemoveAll(filepath.Join(s.Dir, names[0]))
		names = names[1:]
	}
}

func readManifest(dir string) (manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "save.jsonc"))
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := jsonc.Unmarshal(data, &m); err != nil {
		return manifest{}, err
	}
	return m, nil
}

// History says how many saves can be undone and when the newest was made (zero when none).
func (s Store) History() (int, time.Time) {
	names := s.saves()
	if len(names) == 0 {
		return 0, time.Time{}
	}
	m, err := readManifest(filepath.Join(s.Dir, names[len(names)-1]))
	if err != nil {
		return len(names), time.Time{}
	}
	return len(names), m.SavedAt
}

// Undo puts back the files of the newest save - but only if each still holds what that save wrote;
// otherwise nothing is touched and ErrChangedSince comes back. The paths it restored are returned.
func (s Store) Undo() ([]string, error) {
	names := s.saves()
	if len(names) == 0 {
		return nil, ErrNothingToUndo
	}
	dir := filepath.Join(s.Dir, names[len(names)-1])
	m, err := readManifest(dir)
	if err != nil {
		return nil, fmt.Errorf("modedit: the record of the last save cannot be read: %w", err)
	}
	for _, e := range m.Files {
		if !within(e.Path, s.Roots) {
			return nil, fmt.Errorf("modedit: %s is outside the mod's folders", e.Path)
		}
		now, err := os.ReadFile(e.Path)
		if err != nil || sha(now) != e.AfterSHA {
			return nil, fmt.Errorf("%w: %s", ErrChangedSince, e.Path)
		}
	}
	var restored []string
	for _, e := range m.Files {
		if err := restoreEntry(dir, e); err != nil {
			return restored, fmt.Errorf("modedit: restoring %s: %w", e.Path, err)
		}
		restored = append(restored, e.Path)
	}
	os.RemoveAll(dir)
	return restored, nil
}
