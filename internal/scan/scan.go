// Package scan discovers mods for a game: walks its user mod folder,
// classifies and parses each descriptor, and resolves where each mod's real
// content lives (locally, or via Steam Workshop). See docs/mod-sources.md.
package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/steam"
	"github.com/Official-Husko/parallax-mod-manager/internal/xhash"
)

// Options configures a scan.
type Options struct {
	Game game.GameConfig
	// SteamRoots lists every Steam installation root to search (a user can
	// have more than one Steam install - see steam.DefaultRoots). Each is
	// tried in order until one resolves the game's Workshop content
	// directory. If empty (or none resolve), Workshop mods are still
	// discovered from their descriptors, but their ContentPath resolution
	// is skipped (left empty) rather than erroring - a mod manager with no
	// configured Steam root can still show what's there.
	SteamRoots []string
	// ModDir overrides the game's default user mod folder. Tests and
	// callers with an already-resolved directory should set this; production
	// callers normally leave it empty and let it default to
	// opts.Game.UserDataDir() + "/mod".
	ModDir string
	// ExtraFolders lists additional folders a user has pointed Parallax Mod
	// Manager at for more mods beyond the game's own managed mod folder (a
	// shared network drive, a manually curated collection, and the like).
	// Each is searched recursively for self-contained mod folders - see
	// ScanExtraFolder. Only classic-descriptor games are covered.
	ExtraFolders []string
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

// Scan discovers every mod referenced from the game's user mod folder, plus
// (for classic-descriptor games with a resolvable Workshop directory) any
// subscribed Workshop item that doesn't have a linking stub there yet - see
// discoverUnlinkedWorkshopItems.
func Scan(ctx context.Context, opts Options) (Result, error) {
	modDir := opts.ModDir
	if modDir == "" {
		userDir, err := opts.Game.UserDataDir()
		if err != nil {
			return Result{}, err
		}
		modDir = filepath.Join(userDir, "mod")
	}

	// A missing mod folder isn't an error - it just means no mods have
	// been linked into it yet, classic local/Paradox-launcher ones
	// included. Workshop items can still exist independently of it (see
	// below), so scanning continues rather than returning early.
	entries, err := os.ReadDir(modDir)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}

	ext := descriptorExt(opts.Game.DescriptorType)
	var result Result
	knownRemoteIDs := map[string]struct{}{}

	var workshopDir string
	workshopDirResolved := false
	resolveWorkshopDir := func() string {
		if !workshopDirResolved {
			for _, root := range opts.SteamRoots {
				if dir, err := steam.FindWorkshopContentDir(root, opts.Game.SteamAppID); err == nil {
					workshopDir = dir
					break
				}
			}
			workshopDirResolved = true
		}
		return workshopDir
	}

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

		if source == mod.SourceWorkshop {
			if desc.RemoteFileID != "" {
				knownRemoteIDs[desc.RemoteFileID] = struct{}{}
			}
			if dir := resolveWorkshopDir(); dir != "" {
				if resolved := findWorkshopModContentPath(dir, desc); resolved != "" {
					contentPath = resolved
				}
			}
		} else if _, err := os.Stat(contentPath); err != nil {
			// A local mod's declared content path can go stale - the
			// user's mod library moved to a new drive or folder without
			// the stub in modDir being updated to match. Before reporting
			// it missing, check whether any configured extra mod folder
			// now has a same-named subfolder that's plausibly the mod's
			// real content, so pointing Parallax Mod Manager at wherever
			// the library actually lives reconnects it instead of leaving
			// it permanently broken - see findContentByName.
			if found, ok := findContentByName(opts.ExtraFolders, filepath.Base(contentPath)); ok {
				contentPath = found
			}
		}

		newMod := mod.Mod{
			ID:             id,
			Descriptor:     desc,
			Source:         source,
			DescriptorPath: descriptorPath,
			ContentPath:    contentPath,
		}
		if info, statErr := os.Stat(contentPath); statErr != nil || !info.IsDir() {
			newMod.ContentMissing = true
			result.Errors = append(result.Errors, ScanError{Path: descriptorPath, Err: newMod.ContentMissingError()})
		}

		result.Mods = append(result.Mods, newMod)
	}

	// Steam and the Paradox Launcher don't always create a mod/
	// "ugc_<id>.mod" linking stub for a subscribed Workshop item right
	// away - confirmed on a real install with dozens of fully-downloaded
	// items and no stub for any of them (see docs/mod-sources.md). Only
	// classic-descriptor games are covered: every Workshop item's own
	// content folder is confirmed to carry its own self-contained
	// descriptor.mod (no path field needed, since the folder itself is
	// the content) - the JSON-launcher convention for this isn't
	// confirmed, so it's left alone rather than guessed.
	if opts.Game.DescriptorType == mod.DescriptorClassic {
		if dir := resolveWorkshopDir(); dir != "" {
			unlinked, errs := discoverUnlinkedWorkshopItems(dir, knownRemoteIDs)
			result.Mods = append(result.Mods, unlinked...)
			result.Errors = append(result.Errors, errs...)
		}
	}

	for _, folder := range opts.ExtraFolders {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if folder == "" {
			continue
		}
		extra, errs := ScanExtraFolder(opts.Game.DescriptorType, folder)
		result.Mods = append(result.Mods, extra...)
		result.Errors = append(result.Errors, errs...)
	}

	return result, nil
}

// ScanExtraFolder recursively searches root for self-contained mod folders -
// the same "descriptor.mod directly inside the folder that IS the mod's own
// content root" layout Steam Workshop uses (see discoverUnlinkedWorkshopItems)
// - for a user-configured extra mod location outside the game's own managed
// mod folder. Once a folder is recognized as a mod, its own subfolders
// (common/, gfx/, and the like) aren't descended into, so only the
// outermost matching folder on any branch counts - this lets a user point
// at a single mod's folder directly, or at a container of many.
//
// Only classic-descriptor games are covered: the self-contained-folder
// convention isn't confirmed for JSON-descriptor games yet (see
// docs/mod-sources.md) - a no-op (nil, nil) for any other descriptor type,
// same as discoverUnlinkedWorkshopItems' own scoping.
//
// A mod found this way has no linking-stub filename to derive an ID from
// (unlike a classic mod folder's own descriptor stubs, or a Workshop item's
// numbered folder), so its ID is instead derived from its own descriptor
// path via xhash - stable across runs (the same folder always yields the
// same ID) without needing the user to name anything.
func ScanExtraFolder(kind mod.DescriptorType, root string) ([]mod.Mod, []ScanError) {
	if kind != mod.DescriptorClassic || root == "" {
		return nil, nil
	}

	var mods []mod.Mod
	var errs []ScanError
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable branch (permissions, a broken symlink, the
			// root itself missing) just stops descending there - the rest
			// of the tree, if any, is still worth searching.
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		descPath := filepath.Join(path, "descriptor.mod")
		data, err := os.ReadFile(descPath)
		if err != nil {
			return nil // not a mod folder itself - keep descending into it
		}

		desc, err := mod.ParseDescriptor(data, mod.DescriptorClassic)
		if err != nil {
			errs = append(errs, ScanError{Path: descPath, Err: err})
			return fs.SkipDir
		}

		mods = append(mods, mod.Mod{
			ID:             "extra_" + strconv.FormatUint(xhash.Bytes([]byte(descPath)), 36),
			Descriptor:     desc,
			Source:         mod.SourceLocal,
			DescriptorPath: descPath,
			ContentPath:    path,
		})
		return fs.SkipDir
	})
	return mods, errs
}

// discoverUnlinkedWorkshopItems finds every Workshop item under workshopDir
// that isn't already accounted for in knownRemoteIDs (a stub already found
// in the mod folder), parsing each one's own descriptor.mod directly. The
// resulting mod.Mod's ID follows the exact same "ugc_<id>" convention a
// linked stub's filename would give it (see modID), so it round-trips
// correctly through internal/launch's dlc_load.json writer once a stub
// exists - see EnsureWorkshopStub.
func discoverUnlinkedWorkshopItems(workshopDir string, knownRemoteIDs map[string]struct{}) ([]mod.Mod, []ScanError) {
	entries, err := os.ReadDir(workshopDir)
	if err != nil {
		return nil, nil
	}

	var mods []mod.Mod
	var errs []ScanError
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		remoteID := entry.Name()
		if _, known := knownRemoteIDs[remoteID]; known {
			continue
		}

		itemDir := filepath.Join(workshopDir, remoteID)
		descPath := filepath.Join(itemDir, "descriptor.mod")
		data, err := os.ReadFile(descPath)
		if err != nil {
			continue // nothing this project recognizes as a mod - skip quietly
		}

		desc, err := mod.ParseDescriptor(data, mod.DescriptorClassic)
		if err != nil {
			errs = append(errs, ScanError{Path: descPath, Err: err})
			continue
		}
		if desc.RemoteFileID == "" {
			desc.RemoteFileID = remoteID
		}

		mods = append(mods, mod.Mod{
			ID:             mod.WorkshopFilePrefix + remoteID,
			Descriptor:     desc,
			Source:         mod.SourceWorkshop,
			DescriptorPath: descPath,
			ContentPath:    itemDir,
		})
	}
	return mods, errs
}

// EnsureWorkshopStub writes m's descriptor as modDir's linking stub
// ("ugc_<id>.mod") if one doesn't already exist there, so the game (which
// reads dlc_load.json's "mod/ugc_<id>.mod" entries) can actually find
// content this project discovered independently via
// discoverUnlinkedWorkshopItems. Reports whether it wrote a file. Never
// overwrites an existing stub - if Steam or the Paradox Launcher already
// wrote one, that file is left alone untouched. A no-op, not an error, for
// anything that isn't an identifiable Workshop mod.
func EnsureWorkshopStub(m mod.Mod, modDir string) (bool, error) {
	if m.Source != mod.SourceWorkshop || m.Descriptor.RemoteFileID == "" {
		return false, nil
	}
	filename := mod.WorkshopFilePrefix + m.Descriptor.RemoteFileID + ".mod"
	if _, err := os.Stat(filepath.Join(modDir, filename)); err == nil {
		return false, nil
	}

	desc := m.Descriptor
	desc.Path = m.ContentPath
	if _, err := atomicfile.Write(modDir, filename, mod.WriteClassicDescriptor(desc)); err != nil {
		return false, err
	}
	return true, nil
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

// findContentByName searches each of roots recursively for a directory
// literally named target, returning the first match found (roots are tried
// in order; within one root, whichever match filepath.WalkDir reaches
// first - typically shallowest/alphabetically first). Used by Scan to
// reconnect a local mod's stale declared content path to its real folder
// after a user points Parallax Mod Manager at wherever their mod library
// actually lives now (see scan.Options.ExtraFolders) - name matching is a
// heuristic (two unrelated mods could coincidentally share a folder name),
// but it's the only signal available: a moved folder carries no other
// stable identity a stale descriptor could still reference.
func findContentByName(roots []string, target string) (string, bool) {
	if target == "" || target == "." || target == string(filepath.Separator) {
		return "", false
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		var found string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || found != "" {
				return nil
			}
			if d.IsDir() && d.Name() == target {
				found = path
				return fs.SkipAll
			}
			return nil
		})
		if found != "" {
			return found, true
		}
	}
	return "", false
}
