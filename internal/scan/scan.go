// Package scan discovers mods for a game: walks its user mod folder,
// classifies and parses each descriptor, and resolves where each mod's real
// content lives (locally, or via Steam Workshop). See docs/mod-sources.md.
package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/steam"
)

// Options configures a scan.
type Options struct {
	Game game.GameConfig
	// SteamRoot is the Steam installation root (containing steamapps/). If
	// empty, Workshop mods are still discovered from their descriptors, but
	// their ContentPath resolution is skipped (left empty) rather than
	// erroring - a mod manager with no configured Steam root can still show
	// what's there.
	SteamRoot string
	// ModDir overrides the game's default user mod folder. Tests and
	// callers with an already-resolved directory should set this; production
	// callers normally leave it empty and let it default to
	// opts.Game.UserDataDir() + "/mod".
	ModDir string
}

// ScanError records a non-fatal problem with one descriptor - scanning
// continues past it rather than aborting the whole scan.
type ScanError struct {
	Path string
	Err  error
}

func (e ScanError) Error() string { return e.Path + ": " + e.Err.Error() }

// Result is everything a scan produced.
type Result struct {
	Mods   []mod.Mod
	Errors []ScanError
}

// descriptorExt maps a game's descriptor format to the file extension its
// descriptor files use in the mod folder.
func descriptorExt(kind mod.DescriptorType) string {
	switch kind {
	case mod.DescriptorClassic:
		return ".mod"
	default:
		return ".json"
	}
}

// Scan discovers every mod referenced from the game's user mod folder.
func Scan(ctx context.Context, opts Options) (Result, error) {
	modDir := opts.ModDir
	if modDir == "" {
		userDir, err := opts.Game.UserDataDir()
		if err != nil {
			return Result{}, err
		}
		modDir = filepath.Join(userDir, "mod")
	}

	entries, err := os.ReadDir(modDir)
	if os.IsNotExist(err) {
		return Result{}, nil // no mods installed yet is not an error
	}
	if err != nil {
		return Result{}, err
	}

	ext := descriptorExt(opts.Game.DescriptorType)
	var result Result
	var workshopDir string
	workshopDirResolved := false

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			continue
		}

		descriptorPath := filepath.Join(modDir, entry.Name())
		data, err := os.ReadFile(descriptorPath)
		if err != nil {
			result.Errors = append(result.Errors, ScanError{Path: descriptorPath, Err: err})
			continue
		}

		desc, err := mod.ParseDescriptor(data, opts.Game.DescriptorType)
		if err != nil {
			result.Errors = append(result.Errors, ScanError{Path: descriptorPath, Err: err})
			continue
		}

		source := mod.ClassifySource(entry.Name())
		id := modID(entry.Name(), desc, opts.Game.DescriptorType)

		contentPath := desc.Path
		if !filepath.IsAbs(contentPath) {
			contentPath = filepath.Join(modDir, contentPath)
		}

		if source == mod.SourceWorkshop && opts.SteamRoot != "" {
			if !workshopDirResolved {
				workshopDir, _ = steam.FindWorkshopContentDir(opts.SteamRoot, opts.Game.SteamAppID)
				workshopDirResolved = true
			}
			if workshopDir != "" {
				if resolved := findWorkshopModContentPath(workshopDir, desc); resolved != "" {
					contentPath = resolved
				}
			}
		}

		result.Mods = append(result.Mods, mod.Mod{
			ID:             id,
			Descriptor:     desc,
			Source:         source,
			DescriptorPath: descriptorPath,
			ContentPath:    contentPath,
		})
	}

	return result, nil
}

// modID derives a mod's stable identifier: the descriptor filename's stem
// for classic-format mods (which already encodes source, per
// docs/mod-sources.md), or the descriptor's own id for JSON-format mods.
func modID(descriptorFilename string, desc mod.Descriptor, kind mod.DescriptorType) string {
	if kind == mod.DescriptorClassic {
		base := filepath.Base(descriptorFilename)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	if desc.ID != "" {
		return desc.ID
	}
	return descriptorFilename
}

// findWorkshopModContentPath looks for the mod's content under the
// resolved Workshop directory. Each Workshop item is stored in its own
// numbered subfolder (the Steam Workshop file id); the descriptor's
// RemoteFileID is the most reliable way to find it.
func findWorkshopModContentPath(workshopDir string, desc mod.Descriptor) string {
	if desc.RemoteFileID == "" {
		return ""
	}
	candidate := filepath.Join(workshopDir, desc.RemoteFileID)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}
