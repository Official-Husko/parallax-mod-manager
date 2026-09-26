// Package conflict indexes parsed mod content across a load order,
// separates duplicated content from genuine conflicts, applies
// dependency-aware suppression, and resolves each genuine conflict to a
// single winner via a per-Type LIOS/FIOS rule. See
// docs/conflict-resolution.md.
//
// This package is a pure, in-memory transformation: it takes already-parsed
// definitions (the output of pipeline.LoadMod, one call per enabled mod)
// and produces a resolution - it does no file I/O itself. Writing a
// resolved winner out as a physical "patch mod" is a deliberately separate,
// later concern (see docs/conflict-resolution.md's "Patch mods" section).
package conflict

import (
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// LoadOrder is the ordered list of enabled mod IDs for one collection /
// playset. List position IS priority - later entries have higher priority
// - per docs/conflict-resolution.md. It's a plain slice, not a set plus a
// separate order field, so the order is the only source of truth.
type LoadOrder []string

// Priority returns modID's position in the load order (0 = lowest
// priority) and whether it's present at all. If modID appears more than
// once, the first occurrence's position is used.
func (o LoadOrder) Priority(modID string) (pos int, ok bool) {
	for i, id := range o {
		if id == modID {
			return i, true
		}
	}
	return 0, false
}

// Key identifies one conflictable object across every mod: a content
// category (see docs/script-format.md) plus the object's own in-game id.
type Key struct {
	Type definition.Type
	ID   string
}

func keyLess(a, b Key) bool {
	if a.Type != b.Type {
		return a.Type < b.Type
	}
	return a.ID < b.ID
}

// Input is one mod's contribution to a Resolve run: its descriptor (needed
// for dependency-aware suppression) plus its already-parsed Definitions -
// the output of one pipeline.LoadMod call.
type Input struct {
	Mod  mod.Mod
	Defs []definition.Definition
}

// PriorityRule picks a winner among competing definitions for the same
// Key, based on load-order position.
type PriorityRule int

const (
	// LIOS ("Last In, wins") is the default: the candidate whose mod is
	// latest in the load order wins.
	LIOS PriorityRule = iota
	// FIOS ("First In, wins") applies to a minority of object Types, where
	// the earliest mod's version should stick instead.
	FIOS
)

// PriorityRules maps a definition Type to the PriorityRule governing
// conflicts (and same-mod duplicate-key collapsing, which uses the same
// rule) for that Type. A Type absent from the map defaults to LIOS -
// including on a nil PriorityRules, since indexing a nil map is safe in Go.
type PriorityRules map[definition.Type]PriorityRule

// RuleFor returns r[t], or LIOS if t isn't present.
func (r PriorityRules) RuleFor(t definition.Type) PriorityRule {
	return r[t]
}

// DefaultPriorityRules is this project's starting point for which object
// Types use FIOS instead of the LIOS default. Deliberately near-empty:
// treat this as an open, extensible point, not settled data - don't add a
// specific Type name without a confirmed source. A game's Type strings are
// literal mod folder paths (see internal/pipeline's own derivation), so one
// game's entry here can never bleed into another's: Resolve is always
// called for exactly one game's own mod set at a time.
//
// Confirmed so far:
//   - Stellaris' "common/static_modifiers": Paradox's own wiki states a
//     static modifier is overridden by placing the changed version in a
//     new file that sorts *before* the original asciibetically - i.e. the
//     earliest-loaded definition wins, not the latest (fetched live from
//     https://stellaris.paradoxwikis.com/Modifier_modding, 2026-09-24;
//     confirmed only for this one folder - a neighboring one like
//     "common/notification_modifiers" is mentioned on the same page with no
//     such statement, so it is deliberately not assumed to behave the same
//     way).
var DefaultPriorityRules = PriorityRules{
	"common/static_modifiers": FIOS,
}

// EngineMergedTypes is the set of definition Types where the game engine
// itself unions every mod's contribution under the same Key, rather than
// having one mod's version win - see docs/merge-safety-by-type.md. A Key of
// one of these Types is never a genuine conflict, even when 2+ mods define
// it with differing content: the real game runs all of them, it doesn't
// choose one. Deliberately near-empty, the same bar as DefaultPriorityRules
// - don't add a Type without a confirmed source.
//
// Confirmed so far:
//   - Stellaris' "common/on_actions": the real game documents this itself
//     (common/on_actions/99_README_ON_ACTIONS.txt, a real vanilla file, plus
//     Paradox's own wiki: "new entries will be merged with the existing
//     entry with the same NAME={}") - every on_action's events/random_events
//     lists are unioned across every mod that defines the same on_action
//     name, not replaced by whichever loads last.
var EngineMergedTypes = map[definition.Type]bool{
	"common/on_actions": true,
}

// MergeSafeTypes is the set of definition Types where Tier 1 additive
// merging (see docs/merge-patch.md and AdditiveEntries) may even be
// attempted - a losing candidate's own entries that the winner doesn't
// already have get spliced into the generated patch instead of silently
// dropped. Empty by default, the same "don't add a Type without a
// confirmed source" bar as DefaultPriorityRules and EngineMergedTypes: a
// promising-looking shape (see docs/merge-safety-by-type.md's discussion of
// common/scripted_variables) isn't the same thing as a confirmed one, and
// this list should only ever grow from the latter.
var MergeSafeTypes = map[definition.Type]bool{}

// RepeatableMergeKeys names, for a specific Type in MergeSafeTypes, entry
// keys that are confirmed to legitimately repeat within one definition's
// block - so a same-keyed entry with different content from another
// candidate is a new, distinct instance, not a disagreement over the same
// thing. An entry whose key isn't listed here (for its Type) is matched for
// uniqueness instead: only one entry may exist per key, and a same-keyed
// entry with different content is treated as a real disagreement, not a
// safe addition - see AdditiveEntries.
//
// Empty until a Type actually appears in MergeSafeTypes - a repeatable key
// for a Type nothing merges yet has nothing to apply to. Confirmed
// precedent for the one entry worth naming here even before that (see
// docs/merge-safety-by-type.md): Stellaris' "common/governments/authorities",
// key "advanced_authority_swap" - multiple mods' own swap entries are meant
// to coexist, not override each other, confirmed against both a real
// vanilla file and an existing reference implementation's own narrow,
// production auto-merge special case.
var RepeatableMergeKeys = map[definition.Type]map[string]bool{}

// Reason explains why a Resolution's Winner is what it is.
type Reason int

const (
	ReasonSingle     Reason = iota // exactly one mod touches this Key
	ReasonDuplicate                // 2+ mods, identical content - not a conflict
	ReasonSuppressed               // 2+ mods, differing content, but a declared dependency
	// among the competing mods explains it as intentional
	ReasonResolved // a genuine, unsuppressed conflict; Winner chosen by Rule
	// ReasonEngineMerged is 2+ mods, differing content, but the Type is in
	// EngineMergedTypes - the engine merges all of them natively, so this
	// was never a real conflict. Winner is still populated (chosen by Rule,
	// same as any other multi-candidate Key) purely so every Resolution has
	// one; it isn't meaningful here the way it is for ReasonResolved, since
	// nothing about this Key ever "wins" in-game.
	ReasonEngineMerged
)

// DependencyEdge records the declared dependency that caused a conflict to
// be suppressed instead of surfaced.
type DependencyEdge struct {
	FromModID string // the mod that declared the dependency
	ToModID   string // the mod named by that dependency
}

// Resolution is the single winning Definition for one Key, covering every
// Key touched by any input mod - not just the contested ones - since a
// future patch/launch step needs one unambiguous answer per object
// regardless of how it got there.
type Resolution struct {
	Key    Key
	Winner definition.Definition
	Reason Reason
	// Rule is the Type's priority rule; meaningful when Reason is
	// ReasonResolved or ReasonSuppressed.
	Rule PriorityRule
	// Losers is every non-winning candidate, one per competing mod (empty
	// for ReasonSingle) - kept for a future UI/audit view.
	Losers []definition.Definition
	// SuppressedBy is set only when Reason == ReasonSuppressed.
	SuppressedBy *DependencyEdge
}

// Conflict is one genuine, unsuppressed conflict: 2+ mods define Key with
// differing content, and no dependency relationship among them explains it
// as intentional. This is what a future UI surfaces for a manual decision.
type Conflict struct {
	Key        Key
	Candidates []definition.Definition // one per competing mod, ascending load-order position
	Rule       PriorityRule
}

// Options configures a Resolve run.
type Options struct {
	// Rules governs LIOS/FIOS selection. The zero value (nil map) falls
	// back to DefaultPriorityRules.
	Rules PriorityRules
	// ConflictsOnly skips building Result.Resolutions (it stays nil) and
	// only examines keys that more than one mod defines. A caller that only
	// needs Result.Conflicts - which is every caller in the app - saves
	// sorting and resolving every key of every mod, which on a large modlist
	// is several hundred thousand keys, nearly all of them defined by exactly
	// one mod and so never able to conflict. Result.Conflicts is identical
	// either way.
	ConflictsOnly bool
}

// Result is everything one Resolve run produces.
type Result struct {
	Index     Index
	Conflicts []Conflict // sorted by Key; needs user attention
	// Resolutions has every Key touched by any input mod, unless
	// Options.ConflictsOnly was set, in which case it is nil.
	Resolutions map[Key]Resolution
}

// Resolve builds the cross-mod index, detects duplicates/conflicts
// (collapsing a mod's own same-key duplicates first), applies
// dependency-aware suppression, and resolves every Key to a single winning
// Definition per opts.Rules. It performs no I/O.
func Resolve(order LoadOrder, inputs []Input, opts Options) Result {
	rules := opts.Rules
	if rules == nil {
		rules = DefaultPriorityRules
	}

	index := BuildIndex(order, inputs)
	deps := buildDependencyGraph(inputs)

	if opts.ConflictsOnly {
		return Result{Index: index, Conflicts: contestedConflicts(index, deps, order, rules)}
	}

	keys := index.allKeys()
	resolutions := make(map[Key]Resolution, len(keys))
	var conflicts []Conflict

	for _, k := range keys {
		raw, _ := index.At(k)
		res, conflict := detectKey(k, raw, deps, order, rules)
		resolutions[k] = res
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}

	sort.Slice(conflicts, func(i, j int) bool { return keyLess(conflicts[i].Key, conflicts[j].Key) })

	return Result{Index: index, Conflicts: conflicts, Resolutions: resolutions}
}

// contestedConflicts is Resolve's ConflictsOnly path: examine only the keys
// more than one mod defines, run each through the same detectKey the full path
// uses, and return the genuine conflicts sorted by Key. Map iteration order is
// random, so the sort at the end is what makes the result deterministic - and
// equal to the full path's.
func contestedConflicts(index Index, deps dependencyGraph, order LoadOrder, rules PriorityRules) []Conflict {
	var conflicts []Conflict
	index.each(func(k Key, raw []definition.Definition) {
		if singleMod(raw) {
			return
		}
		if _, c := detectKey(k, raw, deps, order, rules); c != nil {
			conflicts = append(conflicts, *c)
		}
	})
	sort.Slice(conflicts, func(i, j int) bool { return keyLess(conflicts[i].Key, conflicts[j].Key) })
	return conflicts
}
