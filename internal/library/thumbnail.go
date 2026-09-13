package library

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
)

// maxThumbnailSize caps how large a thumbnail file ModThumbnail will read -
// a real Workshop preview image is at most a few hundred KB; this guards
// against base64-encoding and shipping something unexpectedly huge across
// the JS boundary as one giant string.
const maxThumbnailSize = 8 * 1024 * 1024

// imageMimeType returns path's MIME type if it's a format a webview can
// render directly, or ok=false otherwise - a classic descriptor's "picture"
// field is occasionally a Paradox-native .dds texture, which isn't
// web-displayable; ModThumbnail skips that rather than trying to convert it.
func imageMimeType(path string) (mimeType string, ok bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".webp":
		return "image/webp", true
	case ".gif":
		return "image/gif", true
	default:
		return "", false
	}
}

// ModThumbnail resolves modID's real thumbnail image, if it has a usable
// one, as a data: URI ready to use directly as an <img> src - the same
// convention internal/gamemedia's caller (app.go's GameMedia) already uses.
// Empty string, no error, means the mod has no usable thumbnail - never
// fabricated.
//
// Resolution order, confirmed against several real Stellaris Workshop mods
// on this machine: the descriptor's own "picture" field first (a
// mod-relative filename the author or Steam chose - "thumb.jpg",
// "thumbnail.png", anything), falling back to a file literally named
// "thumbnail.png" in the mod's content root, since Steam Workshop writes
// one there even for mods whose descriptor never declares a "picture"
// field at all. JSON-format descriptors have no equivalent field yet, so
// only the "thumbnail.png" fallback applies to them.
func ModThumbnail(ctx context.Context, cfg game.GameConfig, opts Options, modID string) (string, error) {
	m, err := findMod(ctx, cfg, opts, modID)
	if err != nil {
		return "", err
	}

	root, err := filepath.Abs(m.ContentPath)
	if err != nil {
		return "", err
	}

	candidates := make([]string, 0, 2)
	if m.Descriptor.Picture != "" {
		candidates = append(candidates, m.Descriptor.Picture)
	}
	candidates = append(candidates, "thumbnail.png")

	for _, rel := range candidates {
		mimeType, ok := imageMimeType(rel)
		if !ok {
			continue
		}
		absPath, err := filepath.Abs(filepath.Join(m.ContentPath, rel))
		if err != nil {
			continue
		}
		// A descriptor's "picture" field is data from the mod itself, not
		// something this project wrote - never trust it to stay inside the
		// mod's own content directory (same guard ReadModFile applies to
		// relPath).
		if absPath != root && !strings.HasPrefix(absPath, root+string(filepath.Separator)) {
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() || info.Size() > maxThumbnailSize {
			continue
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	return "", nil
}
