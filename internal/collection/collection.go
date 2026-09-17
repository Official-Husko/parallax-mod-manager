// Package collection persists named, cross-game groupings of mods - a
// user's own free-form organizing (a "graphics" bucket, a "for my
// campaign" bucket), unlike internal/playset's per-game load orders.
package collection

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

// ModRef identifies one mod within a specific game - a collection's own
// Mods list needs this, not a bare mod ID, since a mod ID is only unique
// within its own game (see internal/mod.Mod.ID's own doc comment) and a
// collection can hold mods from several different games at once.
type ModRef struct {
	GameID string `json:"gameId"`
	ModID  string `json:"modId"`
}

// Collection is one named, cross-game grouping of mods.
type Collection struct {
	Name string   `json:"name"`
	Mods []ModRef `json:"mods"`
}

// FormatVersion guards the on-disk shape - see playset.FormatVersion's own
// doc comment for why a version mismatch is a real error, never a silent
// reset to empty.
const FormatVersion = 1

type storedCollection struct {
	Version int `json:"version"`
	Collection
}

var (
	// ErrDirRequired means Store's directory was never set explicitly -
	// this package never resolves a real path on its own.
	ErrDirRequired = errors.New("collection: Store dir must be set explicitly")
	// ErrNotFound means the named collection doesn't exist.
	ErrNotFound = errors.New("collection: not found")
)

// Store persists collections.
type Store interface {
	// List returns every collection, full data (not just names) - the
	// Library screen's own sidebar shows each one's real mod count
	// alongside its name, and this avoids an N+1 Load per collection just
	// to get it. Sorted by name. A corrupt file is skipped (matching
	// playset.Store.List's own best-effort listing philosophy) rather
	// than failing the whole listing - the real error surfaces when that
	// specific collection is actually Load-ed.
	List(ctx context.Context) ([]Collection, error)
	// Load returns ErrNotFound for a missing collection, and a real error
	// (never a silently-empty Collection) for a corrupt file or a format
	// version this build doesn't understand.
	Load(ctx context.Context, name string) (Collection, error)
	Save(ctx context.Context, c Collection) error
	Delete(ctx context.Context, name string) error
}

// FileStore is the on-disk Store: one JSON file per collection, at
// <Dir>/<sanitized-name>.json, written atomically - no per-game
// subdirectory, unlike playset.FileStore, since a collection isn't scoped
// to one game.
type FileStore struct {
	Dir string
}

func (s FileStore) path(name string) string {
	return filepath.Join(s.Dir, sanitizeFilename(name)+".json")
}

// List reads every *.json file directly under Dir and returns the
// collections found, sorted by name. A missing directory is an empty
// list, not an error - no collections saved yet is the normal starting
// state.
func (s FileStore) List(ctx context.Context) ([]Collection, error) {
	if s.Dir == "" {
		return nil, ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return []Collection{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("collection: listing %s: %w", s.Dir, err)
	}

	// A Go nil slice serializes to JSON null, not [], across the Wails
	// boundary - always return a real empty slice, matching
	// playset.FileStore.List's own precedent.
	collections := []Collection{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			continue
		}
		var stored storedCollection
		if err := json.Unmarshal(data, &stored); err != nil || stored.Name == "" {
			continue
		}
		if stored.Mods == nil {
			stored.Mods = []ModRef{}
		}
		collections = append(collections, stored.Collection)
	}
	sort.Slice(collections, func(i, j int) bool { return collections[i].Name < collections[j].Name })
	return collections, nil
}

// Load reads one named collection.
func (s FileStore) Load(ctx context.Context, name string) (Collection, error) {
	if s.Dir == "" {
		return Collection{}, ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return Collection{}, err
	}

	data, err := os.ReadFile(s.path(name))
	if os.IsNotExist(err) {
		return Collection{}, ErrNotFound
	}
	if err != nil {
		return Collection{}, fmt.Errorf("collection: reading %q: %w", name, err)
	}

	var stored storedCollection
	if err := json.Unmarshal(data, &stored); err != nil {
		return Collection{}, fmt.Errorf("collection: %q is not valid JSON: %w", name, err)
	}
	if stored.Version != FormatVersion {
		return Collection{}, fmt.Errorf("collection: %q has format version %d, this build expects %d", name, stored.Version, FormatVersion)
	}
	if stored.Collection.Mods == nil {
		stored.Collection.Mods = []ModRef{}
	}
	return stored.Collection, nil
}

// Save atomically writes c to disk.
func (s FileStore) Save(ctx context.Context, c Collection) error {
	if s.Dir == "" {
		return ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stored := storedCollection{Version: FormatVersion, Collection: c}
	if _, err := atomicfile.WriteJSON(s.Dir, sanitizeFilename(c.Name)+".json", stored); err != nil {
		return fmt.Errorf("collection: saving %q: %w", c.Name, err)
	}
	return nil
}

// Delete removes one named collection.
func (s FileStore) Delete(ctx context.Context, name string) error {
	if s.Dir == "" {
		return ErrDirRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Remove(s.path(name))
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("collection: deleting %q: %w", name, err)
	}
	return nil
}

// sanitizeFilename mirrors playset.FileStore's own - replaces path
// separators and other filesystem-unsafe characters with "_", so a name
// like "Sci-Fi / Space" works directly as a filename component.
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
