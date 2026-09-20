// Package playset persists named, ordered sets of enabled mods per game -
// Paradox's own term for this concept. Order is the load order (see
// docs/conflict-resolution.md); a mod's absence from ModIDs means disabled,
// not just unprioritized.
package playset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
)

// Playset is one named, ordered, per-game mod selection.
type Playset struct {
	Name        string   `json:"name"`
	GameKey     string   `json:"gameKey"`
	ModIDs      []string `json:"modIds"` // ordered - this is the load order
	DisabledDLC []string `json:"disabledDlc"`
}

// FormatVersion guards the on-disk shape. Unlike internal/cache's
// disposable ModCache, a Playset is irreplaceable user-authored data, so a
// version mismatch or decode error is a real error, never a silent reset to
// empty - see Load's doc comment.
const FormatVersion = 1

type storedPlayset struct {
	Version int `json:"version"`
	Playset
}

var (
	// ErrDirRequired means Store's directory was never set explicitly -
	// this package never resolves a real path on its own.
	ErrDirRequired = errors.New("playset: Store dir must be set explicitly")
	// ErrNotFound means the named playset doesn't exist.
	ErrNotFound = errors.New("playset: not found")
	// ErrExists means a playset already has the name (or, on disk, a file name
	// that sanitizes to the same one) a rename was asked to give another.
	ErrExists = errors.New("playset: a playset with that name already exists")
	// ErrInvalidName means a name was empty once trimmed.
	ErrInvalidName = errors.New("playset: the name can't be empty")
)

// Store persists playsets.
type Store interface {
	// List returns every playset's name for gameKey, best-effort: a
	// corrupt file is skipped (falling back to its filename) rather than
	// failing the whole listing - the real error surfaces when that
	// specific playset is actually Load-ed.
	List(ctx context.Context, gameKey string) ([]string, error)
	// Load returns ErrNotFound for a missing playset, and a real error
	// (never a silently-empty Playset) for a corrupt file or a format
	// version this build doesn't understand.
	Load(ctx context.Context, gameKey, name string) (Playset, error)
	Save(ctx context.Context, p Playset) error
	Delete(ctx context.Context, gameKey, name string) error
	// Rename gives a playset a new name, keeping its mods, order and DLC
	// choices exactly. It fails with ErrNotFound if oldName doesn't exist,
	// ErrExists if newName is already taken, ErrInvalidName for an empty
	// newName - and in every failure case leaves the original untouched.
	Rename(ctx context.Context, gameKey, oldName, newName string) error
}

// FileStore is the on-disk Store: one JSON file per playset, at
// <Dir>/<gameKey>/<sanitized-name>.json, written atomically.
type FileStore struct {
	Dir string
}

func (s FileStore) path(gameKey, name string) string {
	return filepath.Join(s.Dir, gameKey, sanitizeFilename(name)+".json")
}

// List reads every *.json file under <Dir>/<gameKey>/ and returns the
// playset names found, sorted. A missing directory is an empty list, not an
// error - no playsets saved yet is the normal starting state.
func (s FileStore) List(ctx context.Context, gameKey string) ([]string, error) {
	if s.Dir == "" {
		return nil, ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.Dir, gameKey)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("playset: listing %s: %w", dir, err)
	}

	// A Go nil slice serializes to JSON null, not [], across the Wails
	// boundary - always return a real empty slice so the frontend never has
	// to guard against null on top of the usual empty-array case.
	names := []string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			var stored storedPlayset
			if err := json.Unmarshal(data, &stored); err == nil && stored.Name != "" {
				name = stored.Name
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Load reads one named playset.
func (s FileStore) Load(ctx context.Context, gameKey, name string) (Playset, error) {
	if s.Dir == "" {
		return Playset{}, ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return Playset{}, err
	}

	data, err := os.ReadFile(s.path(gameKey, name))
	if os.IsNotExist(err) {
		return Playset{}, ErrNotFound
	}
	if err != nil {
		return Playset{}, fmt.Errorf("playset: reading %q: %w", name, err)
	}

	var stored storedPlayset
	if err := json.Unmarshal(data, &stored); err != nil {
		return Playset{}, fmt.Errorf("playset: %q is not valid JSON: %w", name, err)
	}
	if stored.Version != FormatVersion {
		return Playset{}, fmt.Errorf("playset: %q has format version %d, this build expects %d", name, stored.Version, FormatVersion)
	}
	// A playset file saved before DisabledDLC existed (or one hand-edited
	// to omit it) leaves this field at Go's nil-slice zero value, which
	// crosses the JS boundary as null where the frontend's type says
	// string[] - the same real bug already found once for ModSummary (see
	// library.go's package doc comment).
	if stored.Playset.DisabledDLC == nil {
		stored.Playset.DisabledDLC = []string{}
	}
	if stored.Playset.ModIDs == nil {
		stored.Playset.ModIDs = []string{}
	}
	return stored.Playset, nil
}

// Save atomically writes p to disk.
func (s FileStore) Save(ctx context.Context, p Playset) error {
	if s.Dir == "" {
		return ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(s.Dir, p.GameKey)
	stored := storedPlayset{Version: FormatVersion, Playset: p}
	if _, err := atomicfile.WriteJSON(dir, sanitizeFilename(p.Name)+".json", stored); err != nil {
		return fmt.Errorf("playset: saving %q: %w", p.Name, err)
	}
	return nil
}

// Delete removes one named playset.
func (s FileStore) Delete(ctx context.Context, gameKey, name string) error {
	if s.Dir == "" {
		return ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Remove(s.path(gameKey, name))
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("playset: deleting %q: %w", name, err)
	}
	return nil
}

// Rename implements Store. A playset is a file named after a *sanitized* form
// of its name, so two different names can want the same file ("A/B" and "A_B");
// "already exists" is therefore judged on the file, not the name. A rename that
// only changes what sanitizing would erase anyway (case on a case-insensitive
// disk, say) keeps the same file and just rewrites the name inside it.
//
// The new playset is written before the old one is removed, so a failure part
// way through can never lose the playset: at worst both exist, and if removing
// the old one fails the new one is taken back out again.
func (s FileStore) Rename(ctx context.Context, gameKey, oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return ErrInvalidName
	}
	p, err := s.Load(ctx, gameKey, oldName)
	if err != nil {
		return err
	}
	if p.Name == newName {
		return nil
	}
	samePath := s.path(gameKey, oldName) == s.path(gameKey, newName)
	if !samePath {
		if _, err := os.Stat(s.path(gameKey, newName)); err == nil {
			return ErrExists
		}
	}
	p.Name = newName
	if err := s.Save(ctx, p); err != nil {
		return err
	}
	if samePath {
		return nil
	}
	if err := s.Delete(ctx, gameKey, oldName); err != nil {
		_ = s.Delete(ctx, gameKey, newName)
		return err
	}
	return nil
}

// sanitizeFilename converts a playset name into a safe filename component,
// so names like "Vanilla+ Historical" work directly: replace path
// separators and other filesystem-unsafe characters with "_".
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	sanitized := strings.TrimSpace(b.String())
	if sanitized == "" {
		sanitized = "_"
	}
	return sanitized
}
