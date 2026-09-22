// Package modcheck runs the checks the Editor's Checks tab shows for one mod: syntax errors in
// its own script files, conflicts with the base game, descriptor problems, and dependencies that
// are not installed. See docs/script-format.md and docs/conflict-resolution.md for the concepts
// this builds on - this package only combines already-existing parsing and conflict-resolution
// machinery into per-mod findings, it introduces no new file format handling of its own.
package modcheck

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/locale"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modedit"
	"github.com/Official-Husko/parallax-mod-manager/internal/pipeline"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

// Severity ranks how serious a Finding is.
type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
)

// Category groups a Finding under one of the Checks tab's four sections.
type Category string

const (
	CategoryBaseGame   Category = "base_game"
	CategorySyntax     Category = "syntax"
	CategoryDescriptor Category = "descriptor"
	CategoryDependency Category = "dependency"
)

// Finding is one problem worth showing on the Checks tab.
type Finding struct {
	Category Category
	Severity Severity
	// File is relative to the mod's content folder, or a descriptor
	// filename; "" for a mod-level finding not tied to one file.
	File string
	// Line is 1-based; 0 when not tied to a specific line.
	Line    int
	Message string
}

// vanillaModID is the reserved, synthetic mod ID this package uses to run
// the game's own install directory through the same cache-backed parsing
// pipeline as a real mod (see pipeline.LoadMod) - the same trick
// internal/library's own patchModID uses for the generated patch mod. No
// real mod can ever have this ID: a real ID always comes from an actual
// descriptor's filename stem or its own id field (see internal/scan), never
// a hand-picked literal.
const vanillaModID = "__parallax_vanilla_baseline__"

// Options configures a Check run.
type Options struct {
	// Store is where parsed content is cached - the same store the game's
	// own mod scan uses. Required.
	Store cache.Store
	// InstallDir is the game's real install directory, for the base-game
	// conflict check. Empty skips that check silently - the game not being
	// found isn't this mod's problem to report, and the caller (which
	// already had to resolve this to get here) is in a better position to
	// tell the person why.
	InstallDir string
	// InstalledNames is every other installed mod's display name, for the
	// dependency check - matched exactly, the same way the Editor's own
	// dependency field flags an unknown one as it's typed (see
	// editorDraft.ts's unknownDependencies).
	InstalledNames []string
}

// Check runs every check for one mod and returns its findings, grouped in
// the tab's own fixed category order (base game, syntax, descriptor,
// dependencies) - within a category, by file and then line.
func Check(ctx context.Context, m mod.Mod, cfg game.GameConfig, opts Options) ([]Finding, error) {
	stats := &pipeline.Stats{}
	defs, err := pipeline.LoadMod(ctx, m, cfg, pipeline.Options{Store: opts.Store, Stats: stats})
	if err != nil {
		return nil, fmt.Errorf("modcheck: reading %s's own files: %w", m.ID, err)
	}
	findings := syntaxFindings(m, stats)

	baseGame, err := checkBaseGameConflicts(ctx, m.ID, defs, cfg, opts)
	if err != nil {
		return nil, err
	}

	sortFindings(findings)
	all := append(baseGame, findings...)
	all = append(all, checkDescriptor(m)...)
	all = append(all, checkDependencies(m, opts.InstalledNames)...)
	return all, nil
}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
}

// syntaxFindings turns pipeline.Stats' own per-file problem list into syntax
// Findings. Getting m's Definitions through pipeline.LoadMod (in Check,
// above) rather than a bespoke walk-and-parse means a file whose (mtime,
// size) or content hash hasn't changed since this mod's last scan - by this
// same Check call, or by a routine library scan, or by the Workspace - costs
// no re-parse at all, the same incremental cache internal/conflict's own
// inputs already rely on; only files pipeline.Stats already knows have a
// problem are read again here, to recover the one thing the cache's own
// FileRecord.ParseError (a plain string, kept small and stable on disk)
// doesn't keep: exactly which line.
func syntaxFindings(m mod.Mod, stats *pipeline.Stats) []Finding {
	problems, more := stats.Problems()
	findings := make([]Finding, 0, len(problems))
	for _, p := range problems {
		findings = append(findings, syntaxFindingFor(m, p))
	}
	if more > 0 {
		findings = append(findings, Finding{
			Category: CategorySyntax, Severity: SeverityInfo,
			Message: fmt.Sprintf("%d more file%s with a problem not shown.", more, plural(more)),
		})
	}
	return findings
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// syntaxFindingFor re-reads and re-parses exactly one file already known to
// have a problem, to recover its severity and line: a locale file with only
// some skipped lines is a warning at that line, anything else that still
// fails to parse is an error at the line the parser stopped on. If the file
// now reads fine (changed on disk since the cache noted the problem, but not
// yet rescanned) or can't be read at all, p's own cached message is reported
// as an error with no line, rather than guessing one that may no longer
// apply.
func syntaxFindingFor(m mod.Mod, p pipeline.Problem) Finding {
	data, err := os.ReadFile(filepath.Join(m.ContentPath, p.Path))
	if err != nil {
		return Finding{Category: CategorySyntax, Severity: SeverityError, File: p.Path, Message: p.Err}
	}

	if filepath.Ext(p.Path) == ".yml" {
		if cat, parseErr := locale.Parse(data); parseErr == nil && len(cat.Skipped) > 0 {
			return Finding{Category: CategorySyntax, Severity: SeverityWarn, File: p.Path, Line: cat.Skipped[0].Line, Message: p.Err}
		}
		return Finding{Category: CategorySyntax, Severity: SeverityError, File: p.Path, Message: p.Err}
	}

	if _, parseErr := script.Parse(data); parseErr != nil {
		line := 0
		var syn *script.SyntaxError
		if errors.As(parseErr, &syn) {
			line = syn.Pos.Line
		}
		return Finding{Category: CategorySyntax, Severity: SeverityError, File: p.Path, Line: line, Message: p.Err}
	}
	return Finding{Category: CategorySyntax, Severity: SeverityError, File: p.Path, Message: p.Err}
}

// checkBaseGameConflicts reports every object modID's own files define that
// the base game also defines with different content - the base-game half of
// what the Conflict Resolver finds between mods, but for exactly one mod
// against the vanilla game instead of a whole load order. It runs the
// game's install directory through the same cache-backed pipeline.LoadMod a
// real mod's scan uses (as a synthetic, lowest-priority "mod" - see
// vanillaModID), so a cold first check pays for parsing the whole game once
// and every check after that, for any mod, reuses the warm cache.
func checkBaseGameConflicts(ctx context.Context, modID string, modDefs []definition.Definition, cfg game.GameConfig, opts Options) ([]Finding, error) {
	if opts.InstallDir == "" {
		return nil, nil
	}

	vanilla := mod.Mod{ID: vanillaModID, ContentPath: opts.InstallDir}
	vanillaDefs, err := pipeline.LoadMod(ctx, vanilla, cfg, pipeline.Options{Store: opts.Store})
	if err != nil {
		return nil, fmt.Errorf("modcheck: reading the base game's own files: %w", err)
	}

	order := conflict.LoadOrder{vanillaModID, modID}
	result := conflict.Resolve(order, []conflict.Input{
		{Mod: vanilla, Defs: vanillaDefs},
		{Mod: mod.Mod{ID: modID}, Defs: modDefs},
	}, conflict.Options{ConflictsOnly: true})

	findings := make([]Finding, 0, len(result.Conflicts))
	for _, c := range result.Conflicts {
		for _, cand := range c.Candidates {
			if cand.ModID != modID {
				continue
			}
			findings = append(findings, Finding{
				Category: CategoryBaseGame,
				Severity: SeverityWarn,
				File:     cand.FilePath,
				Line:     cand.Span.StartLine,
				Message:  fmt.Sprintf("Overwrites the base game's own %s %q instead of extending it.", c.Key.Type, c.Key.ID),
			})
		}
	}
	sortFindings(findings)
	return findings, nil
}

// checkDescriptor flags problems with the mod's own saved descriptor: no
// supported_version, one not shaped like the game's own versions, or a
// picture= that names a file which doesn't exist in the mod's folder. It
// checks the descriptor as it actually is on disk, independent of any
// unsaved draft the Editor's Edit tab might be holding.
func checkDescriptor(m mod.Mod) []Finding {
	file := filepath.Base(m.DescriptorPath)
	d := m.Descriptor

	var findings []Finding
	switch sv := strings.TrimSpace(d.SupportedVersion); {
	case sv == "":
		findings = append(findings, Finding{
			Category: CategoryDescriptor, Severity: SeverityWarn, File: file,
			Message: "supported_version is not set, so the game cannot tell if this mod matches its own version.",
		})
	case !modedit.SupportedVersionShape.MatchString(sv):
		findings = append(findings, Finding{
			Category: CategoryDescriptor, Severity: SeverityWarn, File: file,
			Message: fmt.Sprintf("supported_version %q is not shaped like the game's versions (for example v4.*), so the game may treat this mod as made for another version.", sv),
		})
	}

	if d.Picture != "" {
		if _, err := os.Stat(filepath.Join(m.ContentPath, d.Picture)); err != nil {
			findings = append(findings, Finding{
				Category: CategoryDescriptor, Severity: SeverityError, File: file,
				Message: fmt.Sprintf("picture=%q does not exist in the mod's folder.", d.Picture),
			})
		}
	}
	return findings
}

// checkDependencies flags a declared dependency that matches no currently
// installed mod - the same exact-name matching the Editor's own dependency
// field already flags as you type (see editorDraft.ts's unknownDependencies).
func checkDependencies(m mod.Mod, installedNames []string) []Finding {
	installed := make(map[string]bool, len(installedNames))
	for _, n := range installedNames {
		installed[n] = true
	}

	file := filepath.Base(m.DescriptorPath)
	var findings []Finding
	for _, dep := range m.Descriptor.Dependencies {
		dep = strings.TrimSpace(dep)
		if dep == "" || installed[dep] {
			continue
		}
		findings = append(findings, Finding{
			Category: CategoryDependency, Severity: SeverityInfo, File: file,
			Message: fmt.Sprintf("Declares a dependency on %q, which is not currently installed.", dep),
		})
	}
	return findings
}
