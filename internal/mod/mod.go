// Package mod defines the mod descriptor and Mod types, descriptor parsing
// for both formats Paradox games use, and source classification (Workshop /
// Paradox-launcher / local). See docs/paradox-mod-format.md and
// docs/mod-sources.md.
package mod

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DescriptorType selects which descriptor format a game uses. It's a
// per-game property (see internal/game), not something detected per mod.
type DescriptorType int

const (
	DescriptorClassic DescriptorType = iota // descriptor.mod / <name>.mod, Clausewitz key-value
	DescriptorJSONv1                        // Paradox Launcher, v1 relationships (flat strings)
	DescriptorJSONv2                        // Paradox Launcher, v2 relationships ({resource_type, display_name})
)

// Source is where a mod came from, classified by its descriptor's filename -
// see docs/mod-sources.md.
type Source int

const (
	SourceLocal Source = iota
	SourceWorkshop
	SourceParadoxLauncher
	// SourceLoversLab is a mod this app downloaded and installed from LoversLab (see
	// loverslabinstall.go) - unlike Workshop or the Paradox Launcher, nothing external
	// re-manages its files afterward, so it behaves like SourceLocal almost everywhere
	// (editable, eligible for Purge, backed up normally, written as plain "local" in
	// mods_registry.json - a real Paradox-Launcher-format file, not this app's to
	// invent new values for). The one place it's treated distinctly is the update
	// check (a LoversLab file id to compare against, the same way SourceWorkshop's
	// RemoteFileID drives its own) and the Browse-sourced badge shown for it.
	SourceLoversLab
)

const (
	// WorkshopFilePrefix marks a classic-format descriptor filename as a
	// Steam Workshop mod, e.g. "ugc_1830063425.mod" - also the convention
	// this project uses to name a Workshop item's stub file when it has to
	// write one itself (see WriteClassicDescriptor and internal/scan).
	WorkshopFilePrefix = "ugc_"
	launcherFilePrefix = "pdx_"
	// LoversLabFilePrefix marks a classic-format descriptor filename as a mod this
	// app installed from LoversLab, e.g. "loverslab_31347.mod" - the same convention
	// WorkshopFilePrefix uses, just for a different remote source. Deliberately not
	// the shorter "ll_": classification below matches by Contains, not HasPrefix (the
	// same looseness "ugc_"/"pdx_" already accept), and "ll_" is short and common
	// enough to plausibly turn up inside an unrelated local mod's own name (e.g.
	// "shell_script.mod") - "loverslab_" is long and specific enough that collision
	// risk is not a real concern. See loverslabinstall.go.
	LoversLabFilePrefix = "loverslab_"
)

// ClassifySource determines a mod's source from its descriptor's filename
// (e.g. "ugc_1830063425.mod", "pdx_00001.mod", "loverslab_31347.mod", or
// anything else for a local mod). Only the base filename is inspected; any
// directory components in descriptorFilename are ignored.
func ClassifySource(descriptorFilename string) Source {
	name := filepath.Base(descriptorFilename)
	switch {
	case strings.Contains(name, WorkshopFilePrefix):
		return SourceWorkshop
	case strings.Contains(name, launcherFilePrefix):
		return SourceParadoxLauncher
	case strings.Contains(name, LoversLabFilePrefix):
		return SourceLoversLab
	default:
		return SourceLocal
	}
}

// Descriptor holds the fields a mod's metadata file can carry, across both
// the classic Clausewitz format and the JSON formats. Fields only meaningful
// to one format are commented accordingly; the other format simply leaves
// them zero.
type Descriptor struct {
	Name             string
	Path             string
	Version          string
	SupportedVersion string
	Tags             []string
	Dependencies     []string
	ReplacePath      []string
	// Picture is a thumbnail image's filename, relative to the mod's
	// content root - classic format only (e.g. "thumbnail.png",
	// "thumb.jpg"). Often absent even for a real Workshop mod that still
	// ships a "thumbnail.png" Steam writes unconditionally - see
	// internal/library.ModThumbnail, which checks both.
	Picture string

	// RemoteFileID is this mod's id at whatever remote source it came from, classic
	// format only - a Steam Workshop file id for SourceWorkshop, or a LoversLab file
	// id for SourceLoversLab (see loverslabinstall.go). Empty for a mod with no
	// remote source at all.
	RemoteFileID string
	UserDir      string // classic format

	ID               string         // JSON format: the descriptor's own id
	ShortDescription string         // JSON format
	GameCustomData   map[string]any // JSON format: game-specific grab bag (may carry replace_path/user_dir equivalents)
}

// Mod is one discovered, descriptor-parsed mod.
type Mod struct {
	// ID uniquely identifies this mod for this game: for classic-format
	// mods, the descriptor filename's stem (which already encodes source,
	// per docs/mod-sources.md); for JSON-format mods, the descriptor's own
	// id field. Computed by the caller that knows the descriptor's path
	// (see internal/scan), not by this package.
	ID             string
	Descriptor     Descriptor
	Source         Source
	DescriptorPath string // absolute path to the descriptor file
	ContentPath    string // absolute path to the mod's actual content root
	// ContentMissing is true when ContentPath doesn't exist on disk -
	// computed once by internal/scan (which resolves ContentPath in the
	// first place) rather than re-derived independently by everything that
	// later touches a mod's content. A descriptor's declared path can go
	// stale (the mod moved, was renamed, or lives on a drive that isn't
	// currently connected), or a Workshop item's content simply hasn't
	// finished downloading yet - either way, every caller that would
	// otherwise hit a raw, unfriendly filesystem error (internal/library's
	// ListModFiles/ReadModFile/ModThumbnail/GeneratePatch, and
	// resolveConflicts' parsing pass) checks this first instead.
	ContentMissing bool
}

// ContentMissingError explains why m.ContentMissing is true, in plain,
// short language meant to actually be read by a user (not a log) - covers
// both real causes seen in practice, a Workshop item that hasn't finished
// downloading yet or a descriptor's declared path having gone stale,
// without pretending to know which one it is, since ContentMissing itself
// can't distinguish them. Deliberately omits the real ContentPath - a full
// absolute path (confirmed confusing in practice: a drive remounted under
// a different name produces something like
// "/run/media/user/Old Drive Name/...") is debugging detail, not something
// a user needs to see to understand what's wrong. Callers should check
// ContentMissing before calling this; it doesn't verify anything itself.
func (m Mod) ContentMissingError() error {
	name := m.Descriptor.Name
	if name == "" {
		name = m.ID
	}
	if m.Source == SourceWorkshop {
		return fmt.Errorf("%s hasn't finished downloading from Steam Workshop yet", name)
	}
	return fmt.Errorf("%s's content folder can't be found - it may have moved, or its drive isn't connected", name)
}
