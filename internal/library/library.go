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
//
// Likewise, a []string/[]T field here must never be allowed to stay a nil
// slice: encoding/json marshals nil as JSON null, and the generated TS
// binding assigns it straight through (e.g. `this.Tags = source["Tags"]`),
// so a mod with no declared tags/dependencies would otherwise hand the
// frontend a real null where its type says string[] - crashing the first
// .length/.map call against it. See nonNilStrings and ModFiles.Entries'
// construction in modfiles.go.
package library

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/patchmanifest"
	"github.com/Official-Husko/parallax-mod-manager/internal/patchoverride"
	"github.com/Official-Husko/parallax-mod-manager/internal/pipeline"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// GameInfo is a game a user can pick, for a game picker.
type GameInfo struct {
	ID          string
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
	// PathOverridden reports whether InstallPath came from a manually-set
	// path override (see App.gamePathOverride in app.go) rather than
	// automatic Steam-library detection.
	PathOverridden bool
	ModFolder      string // where mods for this game live, whether or not any exist yet
	ModCount       int
}

// DetectGame reports cfg's real install and mod-folder state, using
// cfg.DetectInstall's automatic Steam-library search. steamRoots is used to
// resolve Workshop content for ModCount the same way ScanGame does (see
// scan.Options.SteamRoots) - pass steam.DefaultRoots() in production.
// extraFolders is cfg's configured extra mod folders (see
// scan.Options.ExtraFolders); pass nil if there are none.
func DetectGame(ctx context.Context, cfg game.GameConfig, steamRoots, extraFolders []string) (DetectedGame, error) {
	installDir, installed := cfg.DetectInstall()
	return detectGame(ctx, cfg, installDir, installed, steamRoots, extraFolders)
}

// DetectGameAt reports cfg's state using a caller-supplied install
// directory instead of searching for one - for when a user has manually
// browsed to a game installed somewhere automatic detection doesn't cover.
// The caller is responsible for having already verified installDir with
// cfg.VerifyInstallDir; this never re-checks it, since a manual pick is by
// definition already outside what detection considers valid.
func DetectGameAt(ctx context.Context, cfg game.GameConfig, installDir string, steamRoots, extraFolders []string) (DetectedGame, error) {
	return detectGame(ctx, cfg, installDir, true, steamRoots, extraFolders)
}

// detectGame reports cfg's mod-folder state (always real: scanned fresh)
// alongside the given install info (which the two exported entry points
// resolve differently). A scan failure (e.g. the mod folder doesn't exist
// yet) is not fatal here - it just means ModCount stays 0, matching
// scan.Scan's own "no mods installed yet is not an error" philosophy.
func detectGame(ctx context.Context, cfg game.GameConfig, installDir string, installed bool, steamRoots, extraFolders []string) (DetectedGame, error) {
	userDir, err := cfg.UserDataDir()
	if err != nil {
		return DetectedGame{}, fmt.Errorf("library: resolving user data dir for %s: %w", cfg.ID, err)
	}
	modFolder := filepath.Join(userDir, "mod")

	modCount := 0
	if result, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: steamRoots, ExtraFolders: extraFolders}); err == nil {
		modCount = len(result.Mods)
	}

	return DetectedGame{
		GameInfo:    GameInfo{ID: cfg.ID, DisplayName: cfg.DisplayName},
		Installed:   installed,
		InstallPath: installDir,
		ModFolder:   modFolder,
		ModCount:    modCount,
	}, nil
}

// ModSummary is one mod, summarized for display.
type ModSummary struct {
	ID               string
	Name             string
	Version          string
	SupportedVersion string // the descriptor's own compatibility claim, e.g. "3.9.*" - never verified against the actual installed game version
	Source           string // "local" | "workshop" | "paradox-launcher"
	Tags             []string
	Dependencies     []string // other mods' names this one declares it expects to load before it - names only, per docs/paradox-mod-format.md, not resolved to another mod's ID
	// RemoteFileID is the Steam Workshop file id, non-empty only for
	// Source == "workshop" - enough for a caller to link to the mod's real
	// Workshop page without this project fetching anything itself.
	RemoteFileID string
	// ShortDescription is only ever populated for JSON-format descriptors
	// (see docs/paradox-mod-format.md) - the classic format has no
	// description field at all, so this stays empty for those rather than
	// inventing one.
	ShortDescription string
	// Enabled reflects membership in the Options.Order used for this
	// LoadGame call - a disabled mod is listed here but was never parsed
	// or considered for conflicts (see LoadGame).
	Enabled bool
	// GeneratedPatch is true for the one mod this project itself generates
	// (see GeneratePatch) - lets the frontend give it special treatment
	// (Autosort keeps it last, dependency checks leave it alone) without
	// knowing its ID.
	GeneratedPatch bool
}

// GameUpdate is a game whose installed version changed since the app last saw
// it - see App.CheckGameUpdates. From and To are the versions as the game
// reports them (e.g. "v4.4.5").
type GameUpdate struct {
	GameID   string
	GameName string
	From     string
	To       string
}

// ConflictCandidate is one mod competing for a contested Type+ID pair,
// summarized for display.
type ConflictCandidate struct {
	ModID    string
	ModName  string
	FilePath string // mod-relative - the file the competing definition came from
}

// ConflictSummary is one genuine, unresolved conflict, summarized for
// display.
type ConflictSummary struct {
	Type       string
	ID         string
	Candidates []ConflictCandidate // ascending load-order position
	// Winner is the ModID of the candidate that actually applies in-game:
	// a manual override from Options.Overrides, if one names a mod that's
	// still a real candidate for this key, otherwise the automatic
	// winner per Rule (last-in wins for LIOS, first-in wins for FIOS) -
	// the same load-order semantics conflict.Resolve itself already
	// applies. See docs/patch-mods.md.
	Winner string
	// Overridden is true when Winner came from a manual override rather
	// than the automatic load-order rule - lets the UI show a conflict
	// the user has explicitly decided on differently from one still using
	// the default.
	Overridden bool
	// PatchState says how this key stands against the generated patch
	// mod: "" when no patch has been generated, otherwise one of the
	// PatchState* constants. See applyPatchState.
	PatchState string
	// PatchNote is a plain-language explanation of PatchState when it's
	// "changed" or "new" (what changed, and in which mods) - empty
	// otherwise.
	PatchNote string
}

// Summary is everything a LoadGame call produces.
type Summary struct {
	Game      GameInfo
	Mods      []ModSummary
	Conflicts []ConflictSummary
	// Patch describes the generated patch mod's freshness against this
	// scan's conflicts. Zero value (Exists false) when there's no patch.
	Patch PatchSummary
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
	// SteamRoots is optional; empty skips Workshop content-path resolution
	// (see scan.Options.SteamRoots).
	SteamRoots []string
	// ModDir is an optional test/override hook mirroring
	// scan.Options.ModDir; empty uses the game's real mod folder.
	ModDir string
	// ExtraFolders mirrors scan.Options.ExtraFolders - a user's configured
	// extra mod locations for this game, searched recursively alongside
	// the game's own managed mod folder.
	ExtraFolders []string
	// Order, if set, both determines load order and which mods are
	// enabled - a mod absent from it is disabled: excluded from parsing
	// and conflict detection entirely (see LoadGame), and marked
	// Enabled: false in the summary. nil means "no selection made yet" -
	// every scanned mod starts disabled (an empty load order), the same
	// "nothing pre-chosen, managing a mod is an explicit choice" default
	// the first-run wizard already uses for games.
	Order conflict.LoadOrder
	// Overrides maps a conflict's patchoverride.Key (its "Type:ID") to a
	// ModID manually chosen to win it, overriding the automatic
	// load-order winner conflict.Resolve would otherwise pick - see
	// docs/patch-mods.md. nil (the zero value) means no manual overrides;
	// an override naming a mod that isn't actually a candidate for that
	// key (removed, disabled, or simply never a real one) is ignored
	// rather than erroring, falling back to the automatic winner.
	Overrides map[string]string
	// OnQuickSummary, if set, is called once every mod is known (right
	// after scanning, before any of the slow per-mod content parsing that
	// conflict detection needs) with a Summary that already has every
	// mod's name/version/source/enabled state but an empty Conflicts list -
	// letting a caller show the mod list immediately instead of waiting for
	// parsing to finish. The final return value is always the complete,
	// authoritative Summary; this is purely an earlier, partial preview.
	OnQuickSummary func(Summary)
	// ContentPaths, if set, is filled with every scanned mod's content
	// directory as a side effect of LoadGame, and consulted by ReadModFile
	// so a single-file read doesn't need a full re-scan to find its mod -
	// see ContentPathCache. nil disables both.
	ContentPaths *ContentPathCache
	// GameVersion is the installed game's real version (e.g. "v4.4.6"), if
	// known. GeneratePatch turns it into the patch mod's own
	// supported_version so the launcher doesn't flag the patch as
	// out-of-date; empty falls back to "*" (any version).
	GameVersion string
	// PatchThumbnail is the PNG GeneratePatch writes into a new patch mod as
	// its thumbnail (and names as the descriptor's picture). Empty means the
	// patch is written without one.
	PatchThumbnail []byte
}

// maxModWorkers caps how many mods are read at once. More than this stops
// helping (the work per mod is small once its cache is warm) and only raises
// how much is in memory at the same moment on a cold run.
const maxModWorkers = 8

// resolvedGame is the raw, unsummarized output of scanning, parsing, and
// resolving conflicts for one game - shared by LoadGame (which summarizes
// it for the frontend) and GeneratePatch (which needs each conflict
// candidate's original mod.Mod, in particular ContentPath, and the raw
// conflict.Conflict data ConflictSummary deliberately doesn't expose, like
// each candidate's byte-range Span).
type resolvedGame struct {
	mods         []mod.Mod
	modSummaries []ModSummary
	names        map[string]string
	result       conflict.Result
	errs         []string
	// modsRead is how many enabled mods were actually read for conflict
	// detection. Zero means "no load order to judge", not "no conflicts".
	modsRead int
}

// resolveConflicts scans cfg's mod folder, parses every *enabled* mod
// through the existing cache-backed pipeline, and resolves conflicts using
// opts.Order (or, absent one, nothing enabled) - the pipeline LoadGame and
// GeneratePatch both need.
func resolveConflicts(ctx context.Context, cfg game.GameConfig, opts Options) (resolvedGame, error) {
	scanResult, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: opts.SteamRoots, ModDir: opts.ModDir, ExtraFolders: opts.ExtraFolders})
	if err != nil {
		return resolvedGame{}, fmt.Errorf("library: scanning %s: %w", cfg.ID, err)
	}

	mods := append([]mod.Mod(nil), scanResult.Mods...)
	sort.Slice(mods, func(i, j int) bool { return mods[i].ID < mods[j].ID })
	opts.ContentPaths.Store(cfg.ID, mods)

	var errs []string
	for _, se := range scanResult.Errors {
		errs = append(errs, se.Error())
	}

	order := opts.Order
	if order == nil {
		order = conflict.LoadOrder{}
	}
	enabled := make(map[string]bool, len(order))
	for _, id := range order {
		enabled[id] = true
	}

	// Every mod's name/version/source is already known from its descriptor
	// alone - no content parsing needed - so the full mod list can be built,
	// and handed to opts.OnQuickSummary, before any of the slow per-mod
	// parsing below even starts.
	names := make(map[string]string, len(mods))
	modSummaries := make([]ModSummary, 0, len(mods))
	for _, m := range mods {
		names[m.ID] = displayName(m)
		modSummaries = append(modSummaries, ModSummary{
			ID:               m.ID,
			Name:             displayName(m),
			Version:          m.Descriptor.Version,
			SupportedVersion: m.Descriptor.SupportedVersion,
			Source:           sourceString(m.Source),
			Tags:             nonNilStrings(m.Descriptor.Tags),
			Dependencies:     nonNilStrings(m.Descriptor.Dependencies),
			ShortDescription: m.Descriptor.ShortDescription,
			RemoteFileID:     m.Descriptor.RemoteFileID,
			Enabled:          enabled[m.ID],
			GeneratedPatch:   m.ID == patchModID,
		})
	}

	if opts.OnQuickSummary != nil {
		opts.OnQuickSummary(Summary{
			Game:   GameInfo{ID: cfg.ID, DisplayName: cfg.DisplayName},
			Mods:   append([]ModSummary(nil), modSummaries...),
			Errors: append([]string(nil), errs...),
		})
	}

	// Decide which mods actually get parsed.
	var toLoad []mod.Mod
	for _, m := range mods {
		if m.ID == patchModID {
			// The generated patch is an *output* of conflict resolution, not
			// a competitor in it: it repeats the winner's text for every key
			// it covers, so parsing it here would make it a candidate (and,
			// being last in the load order, the winner) of every one of
			// those conflicts, and there'd be no way left to tell what the
			// real mods say from what the patch says they should. It's still
			// listed as a mod, and still loads in the game. See
			// docs/patch-mods.md.
			continue
		}
		if !enabled[m.ID] {
			// A disabled mod is never parsed: it can't contribute to a
			// conflict it isn't loaded for, and skipping the parse
			// entirely is a real performance win, not just a correctness
			// detail (see docs/performance-strategy.md).
			continue
		}
		if m.ContentMissing {
			// scan.Scan already recorded a clear, human-readable error for
			// this in errs above (via scanResult.Errors) - attempting to
			// parse it here would only produce a second, worse message
			// (a raw filesystem error) for the exact same problem.
			continue
		}
		toLoad = append(toLoad, m)
	}

	// Parse them, several mods at a time. Each mod's own files are already
	// parsed in parallel inside pipeline.LoadMod, but a warm run is dominated
	// by something that isn't per-file: reading and decoding each mod's cache
	// file, one after another. Loading a few mods at once overlaps those, and
	// lets a run of small mods proceed while one huge mod is still working.
	// Results land in a slot per mod and are merged below in the original
	// order, so what the conflict logic sees - and so which mod wins - is
	// exactly what a sequential load would have produced (see
	// docs/performance-strategy.md: parse in parallel, merge sequentially).
	type loaded struct {
		defs []definition.Definition
		err  error
	}
	results := make([]loaded, len(toLoad))
	stats := &pipeline.Stats{}
	loadTimer := applog.For("Cache").Begin()
	modWorkers := min(runtime.NumCPU(), maxModWorkers)
	fileWorkers := max(1, runtime.NumCPU()/modWorkers) // keep the total near the core count
	var wg sync.WaitGroup
	sem := make(chan struct{}, modWorkers)
	for i, m := range toLoad {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defs, err := pipeline.LoadMod(ctx, m, cfg, pipeline.Options{
				Store:   cache.FileStore{Dir: opts.CacheDir},
				Workers: fileWorkers,
				Stats:   stats,
			})
			results[i] = loaded{defs: defs, err: err}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return resolvedGame{}, err
	}

	var inputs []conflict.Input
	totalDefs := 0
	for i, m := range toLoad {
		if results[i].err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.ID, results[i].err))
			continue
		}
		inputs = append(inputs, conflict.Input{Mod: m, Defs: results[i].defs})
		totalDefs += len(results[i].defs)
	}
	if len(toLoad) > 0 {
		loadTimer.Infof("read %d mods: %d files (%d cached, %d re-checked, %d parsed, %d with parse problems), %d cache files rewritten",
			len(toLoad), stats.Files.Load(), stats.Cached.Load(), stats.Touched.Load(), stats.Parsed.Load(), stats.ParseErrors.Load(), stats.Saved.Load())
		logParseProblems(applog.For("Cache"), stats, names)
	}

	resolveTimer := applog.For("Conflicts").Begin()
	// Only the conflicts are used from here on, so the per-key resolution of
	// every uncontested definition is skipped - see conflict.Options.
	result := conflict.Resolve(order, inputs, conflict.Options{ConflictsOnly: true})
	resolveTimer.Infof("%d contested keys among %d definitions from %d mods", len(result.Conflicts), totalDefs, len(inputs))

	return resolvedGame{mods: mods, modSummaries: modSummaries, names: names, result: result, errs: errs, modsRead: len(inputs)}, nil
}

// logParseProblems names the files stats found a problem with, so the one-line
// "N with parse problems" summary above it can be acted on: which mod, which
// file, what was wrong. Only files that were parsed by this very run are named
// (see pipeline.Stats.Problems), so a mod's broken file is reported when it is
// first seen or changes, not on every scan.
func logParseProblems(log applog.Logger, stats *pipeline.Stats, names map[string]string) {
	problems, more := stats.Problems()
	for _, p := range problems {
		name := names[p.ModID]
		if name == "" {
			name = p.ModID
		}
		log.Warnf("couldn't fully read '%s' in mod '%s': %s", p.Path, name, clipText(p.Err, maxProblemText))
	}
	if more > 0 {
		log.Warnf("... and %d more files with problems", more)
	}
}

// maxProblemText is the longest reason logParseProblems writes in full. A
// localisation error quotes the whole offending line, which can run to
// hundreds of characters.
const maxProblemText = 160

// clipText shortens s to at most limit characters (not bytes - it never cuts a
// multi-byte character in half), marking the cut with an ellipsis.
func clipText(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

// LoadGame scans cfg's mod folder, parses every *enabled* mod through the
// existing cache-backed pipeline, resolves conflicts using opts.Order (or,
// absent one, nothing enabled), and summarizes the result.
func LoadGame(ctx context.Context, cfg game.GameConfig, opts Options) (Summary, error) {
	rg, err := resolveConflicts(ctx, cfg, opts)
	if err != nil {
		return Summary{}, err
	}
	conflicts := buildConflictSummaries(rg.result.Conflicts, rg.names, opts.Overrides)
	patch := PatchSummary{ChangedMods: []string{}}
	if dir, ok := patchContentDir(cfg, opts); ok {
		if manifest, ok := patchmanifest.Load(dir); ok {
			if rg.modsRead == 0 {
				// Nothing was read (no playset chosen yet, or none of its mods
				// could be), so there are no conflicts to compare the patch
				// with - and "every key it covers has stopped conflicting" would
				// be a false alarm about a load order that simply isn't loaded.
				patch = describePatch(manifest, opts.GameVersion)
			} else {
				patch = applyPatchState(conflicts, rg.result.Conflicts, manifest, rg.names, opts.GameVersion)
			}
		}
	}
	return Summary{
		Game:      GameInfo{ID: cfg.ID, DisplayName: cfg.DisplayName},
		Mods:      rg.modSummaries,
		Conflicts: conflicts,
		Patch:     patch,
		Errors:    rg.errs,
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

// nonNilStrings returns s unchanged if non-nil, or a real empty (non-nil)
// slice otherwise. A descriptor with no declared tags/dependencies leaves
// the corresponding field as Go's nil-slice zero value, which encoding/json
// marshals as JSON null rather than []; the generated TS binding assigns
// that straight through (`this.Tags = source["Tags"]`), so without this the
// frontend gets a real null where its type says string[] and crashes the
// first time it calls .length/.map on it - see this package's doc comment.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// sourceString converts mod.Source's iota enum to a stable string safe to
// cross the JS boundary - see this package's doc comment.
func sourceString(s mod.Source) string {
	switch s {
	case mod.SourceWorkshop:
		return "workshop"
	case mod.SourceParadoxLauncher:
		return "paradox-launcher"
	case mod.SourceLoversLab:
		return "loverslab"
	default:
		return "local"
	}
}

// buildConflictSummaries maps conflict.Conflicts to ConflictSummary,
// resolving each candidate's ModID to a display name via names and the
// effective winner (a manual override from overrides, or the automatic
// one per c.Rule) via effectiveWinner.
func buildConflictSummaries(conflicts []conflict.Conflict, names map[string]string, overrides map[string]string) []ConflictSummary {
	summaries := make([]ConflictSummary, 0, len(conflicts))
	for _, c := range conflicts {
		candidates := make([]ConflictCandidate, 0, len(c.Candidates))
		for _, d := range c.Candidates {
			name, ok := names[d.ModID]
			if !ok {
				name = d.ModID
			}
			candidates = append(candidates, ConflictCandidate{ModID: d.ModID, ModName: name, FilePath: d.FilePath})
		}
		winner, overridden := effectiveWinner(c, overrides)
		summaries = append(summaries, ConflictSummary{
			Type:       string(c.Key.Type),
			ID:         c.Key.ID,
			Candidates: candidates,
			Winner:     winner,
			Overridden: overridden,
		})
	}
	return summaries
}

// winnerModID returns the ModID of c's automatically-applying candidate
// per c.Rule (last-in wins for LIOS, first-in wins for FIOS -
// c.Candidates is already ascending load-order position). Empty if c has
// no candidates.
func winnerModID(c conflict.Conflict) string {
	if len(c.Candidates) == 0 {
		return ""
	}
	if c.Rule == conflict.FIOS {
		return c.Candidates[0].ModID
	}
	return c.Candidates[len(c.Candidates)-1].ModID
}

// effectiveWinner returns c's real, applying winner: overrides[Key(c)] if
// it names a mod that's still genuinely one of c's own candidates,
// otherwise winnerModID's automatic pick. An override naming a mod
// that's been removed, disabled, or was never actually a candidate for
// this key is silently ignored rather than erroring - a stale override
// falling back to the automatic winner is the same safe behavior a
// missing override already has, not a new failure mode to guard against
// separately. The second return value reports which case happened, for
// ConflictSummary.Overridden.
func effectiveWinner(c conflict.Conflict, overrides map[string]string) (winner string, overridden bool) {
	if chosen, ok := overrides[patchoverride.Key(string(c.Key.Type), c.Key.ID)]; ok {
		for _, cand := range c.Candidates {
			if cand.ModID == chosen {
				return chosen, true
			}
		}
	}
	return winnerModID(c), false
}
