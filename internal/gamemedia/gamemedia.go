// Package gamemedia resolves per-game art (a logo, a background) by the
// game's UUID, the same embed-plus-live-override pattern internal/game
// uses for the games list itself: a compiled-in default, overridable by a
// real file dropped in the user's config directory, with no rebuild
// needed to add or replace one.
package gamemedia

import (
	"io/fs"
	"os"
	"path/filepath"
)

// extensions lists the file extensions Find checks, in the order tried.
var extensions = []string{"png", "svg", "jpg", "jpeg", "webp"}

// Store resolves one named media file per game. Embedded should already be
// rooted at the media directory itself (e.g. via fs.Sub) so a lookup path
// is just "<kind>/<gameID>.<ext>" - the caller wires up exactly what
// "compiled-in default" means, this package only does the lookup.
type Store struct {
	Embedded    fs.FS  // compiled-in defaults; nil disables this lookup
	OverrideDir string // real on-disk directory checked first; "" disables it
}

// Find looks for <kind>/<gameID>.<ext> (kind e.g. "logo" or "background"),
// checking s.OverrideDir first and s.Embedded second. Not finding one
// anywhere is not an error - it just means no art exists yet for that
// game, matching this project's "missing is fine, don't fabricate"
// handling everywhere else (scan.Scan, playset.List, etc.).
func (s Store) Find(kind, gameID string) (data []byte, mimeType string, ok bool) {
	if s.OverrideDir != "" {
		for _, ext := range extensions {
			path := filepath.Join(s.OverrideDir, kind, gameID+"."+ext)
			if data, err := os.ReadFile(path); err == nil {
				return data, mimeTypeForExt(ext), true
			}
		}
	}

	if s.Embedded != nil {
		for _, ext := range extensions {
			// fs.FS paths always use forward slashes, regardless of OS.
			path := kind + "/" + gameID + "." + ext
			if data, err := fs.ReadFile(s.Embedded, path); err == nil {
				return data, mimeTypeForExt(ext), true
			}
		}
	}

	return nil, "", false
}

func mimeTypeForExt(ext string) string {
	switch ext {
	case "png":
		return "image/png"
	case "svg":
		return "image/svg+xml"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
