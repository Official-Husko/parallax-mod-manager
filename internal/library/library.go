// Package library orchestrates a full "scan this game, parse its mods,
// resolve conflicts" run using the existing scan/pipeline/conflict
// packages, and summarizes the result for the frontend.
//
// The summary types deliberately never carry a raw uint64/int64 field or a
// bare Go enum value: Wails maps both straight to a plain JS number
// (verified against the installed Wails v2 source's TypeScript generator),
// which either silently loses precision (a 64-bit hash routinely exceeds
// JS's 2^53 safe-integer range) or arrives meaningless (an iota has no
// semantics on the JS side). Keep it that way - convert to a string at this
// boundary rather than passing a raw numeric field through.
package library

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/pipeline"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// GameInfo is a game a user can pick, for a game picker.
type GameInfo struct {
	Key         string
	DisplayName string
}

// DetectedGame is one registered game's real, on-this-machine setup state,
// for the first-run wizard's "games found" step - never fabricated: a game
// with no confirmed Steam install reports Installed: false and an empty
// InstallPath rather than guessing.
type DetectedGame struct {
	GameInfo
	Installed   bool
	InstallPath string // empty when Installed is false
	ModFolder   string // where mods for this game live, whether or not any exist yet
	ModCount    int
}

// DetectGame reports cfg's real install and mod-folder state. A scan
// failure (e.g. the mod folder doesn't exist yet) is not fatal here - it
// just means ModCount stays 0, matching scan.Scan's own "no mods installed
// yet is not an error" philosophy.
func DetectGame(ctx context.Context, cfg game.GameConfig) (DetectedGame, error) {
	installDir, installed := cfg.DetectInstall()

	userDir, err := cfg.UserDataDir()
	if err != nil {
		return DetectedGame{}, fmt.Errorf("library: resolving user data dir for %s: %w", cfg.Key, err)
	}
	modFolder := filepath.Join(userDir, "mod")

	modCount := 0
	if result, err := scan.Scan(ctx, scan.Options{Game: cfg}); err == nil {
		modCount = len(result.Mods)
	}

	return DetectedGame{
		GameInfo:    GameInfo{Key: cfg.Key, DisplayName: cfg.DisplayName},
		Installed:   installed,
		InstallPath: installDir,
		ModFolder:   modFolder,
		ModCount:    modCount,
	}, nil
}

// ModSummary is one mod, summarized for display.
type ModSummary struct {
	ID      string
	Name    string
	Version string
	Source  string // "local" | "workshop" | "paradox-launcher"
	Tags    []string
	// Enabled reflects membership in the Options.Order used for this
	// LoadGame call - a disabled mod is listed here but was never parsed
	// or considered for conflicts (see LoadGame).
	Enabled bool
}

// ConflictSummary is one genuine, unresolved conflict, summarized for
// display.
type ConflictSummary struct {
	Type       string
	ID         string
	Candidates []string // competing mods' display names, ascending load-order position
}

// Summary is everything a LoadGame call produces.
type Summary struct {
	Game      GameInfo
	Mods      []ModSummary
	Conflicts []ConflictSummary
	// Errors collects non-fatal problems (a bad descriptor, a mod that
	// failed to parse) as strings - one bad mod must not abort the whole
	// scan, matching scan.go's and pipeline.go's own established
	// non-fatal-per-item philosophy.
	Errors []string
}

// Options configures a LoadGame call. Real paths are resolved by the
// caller (app.go), never by this package - the same rule internal/launch
// already established for the same reason.
type Options struct {
	// CacheDir is required - passed to pipeline's cache.FileStore.
	CacheDir string
	// SteamRoot is optional; empty skips Workshop content-path resolution
	// (see scan.Options.SteamRoot).
	SteamRoot string
	// ModDir is an optional test/override hook mirroring
	// scan.Options.ModDir; empty uses the game's real mod folder.
	ModDir string
	// Order, if set, both determines load order and which mods are
	// enabled - a mod absent from it is disabled: excluded from parsing
	// and conflict detection entirely (see LoadGame), and marked
	// Enabled: false in the summary. nil means "no selection made yet" -
	// every scanned mod is enabled, ID-sorted (the original placeholder
	// behavior, kept as the default for a fresh, playset-less scan).
	Order conflict.LoadOrder
}

// LoadGame scans cfg's mod folder, parses every *enabled* mod through the
// existing cache-backed pipeline, resolves conflicts using opts.Order (or,
// absent one, every mod enabled and ID-sorted), and summarizes the result.
func LoadGame(ctx context.Context, cfg game.GameConfig, opts Options) (Summary, error) {
	scanResult, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoot: opts.SteamRoot, ModDir: opts.ModDir})
	if err != nil {
		return Summary{}, fmt.Errorf("library: scanning %s: %w", cfg.Key, err)
	}

	mods := append([]mod.Mod(nil), scanResult.Mods...)
	sort.Slice(mods, func(i, j int) bool { return mods[i].ID < mods[j].ID })

	var errs []string
	for _, se := range scanResult.Errors {
		errs = append(errs, se.Error())
	}

	order := opts.Order
	if order == nil {
		order = make(conflict.LoadOrder, 0, len(mods))
		for _, m := range mods {
			order = append(order, m.ID)
		}
	}
	enabled := make(map[string]bool, len(order))
	for _, id := range order {
		enabled[id] = true
	}

	names := make(map[string]string, len(mods))
	modSummaries := make([]ModSummary, 0, len(mods))
	var inputs []conflict.Input

	for _, m := range mods {
		names[m.ID] = displayName(m)
		isEnabled := enabled[m.ID]
		modSummaries = append(modSummaries, ModSummary{
			ID:      m.ID,
			Name:    displayName(m),
			Version: m.Descriptor.Version,
			Source:  sourceString(m.Source),
			Tags:    m.Descriptor.Tags,
			Enabled: isEnabled,
		})

		if !isEnabled {
			// A disabled mod is never parsed: it can't contribute to a
			// conflict it isn't loaded for, and skipping the parse
			// entirely is a real performance win, not just a correctness
			// detail (see docs/performance-strategy.md).
			continue
		}

		defs, err := pipeline.LoadMod(ctx, m, cfg, pipeline.Options{Store: cache.FileStore{Dir: opts.CacheDir}})
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.ID, err))
			continue
		}
		inputs = append(inputs, conflict.Input{Mod: m, Defs: defs})
	}

	result := conflict.Resolve(order, inputs, conflict.Options{})

	return Summary{
		Game:      GameInfo{Key: cfg.Key, DisplayName: cfg.DisplayName},
		Mods:      modSummaries,
		Conflicts: buildConflictSummaries(result.Conflicts, names),
		Errors:    errs,
	}, nil
}

// displayName returns a mod's human-readable name, falling back to its ID
// when the descriptor didn't provide one.
func displayName(m mod.Mod) string {
	if m.Descriptor.Name != "" {
		return m.Descriptor.Name
	}
	return m.ID
}

// sourceString converts mod.Source's iota enum to a stable string safe to
// cross the JS boundary - see this package's doc comment.
func sourceString(s mod.Source) string {
	switch s {
	case mod.SourceWorkshop:
		return "workshop"
	case mod.SourceParadoxLauncher:
		return "paradox-launcher"
	default:
		return "local"
	}
}

// buildConflictSummaries maps conflict.Conflicts to ConflictSummary,
// resolving each candidate's ModID to a display name via names.
func buildConflictSummaries(conflicts []conflict.Conflict, names map[string]string) []ConflictSummary {
	summaries := make([]ConflictSummary, 0, len(conflicts))
	for _, c := range conflicts {
		candidates := make([]string, 0, len(c.Candidates))
		for _, d := range c.Candidates {
			if name, ok := names[d.ModID]; ok {
				candidates = append(candidates, name)
			} else {
				candidates = append(candidates, d.ModID)
			}
		}
		summaries = append(summaries, ConflictSummary{
			Type:       string(c.Key.Type),
			ID:         c.Key.ID,
			Candidates: candidates,
		})
	}
	return summaries
}
