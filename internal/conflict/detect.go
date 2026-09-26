package conflict

import (
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// detectKey resolves one Key's raw candidates (as returned by
// Index.At(k)) into a single Resolution, and - only when the Key is a
// genuine, unsuppressed conflict - the Conflict describing it.
func detectKey(k Key, raw []definition.Definition, deps dependencyGraph, order LoadOrder, rules PriorityRules) (Resolution, *Conflict) {
	rule := rules.RuleFor(k.Type)

	// Collapse each mod's own same-key duplicates (e.g. two of its own
	// files defining the same object) down to one representative per mod,
	// using the same rule a cross-mod conflict at this Type would use -
	// the Clausewitz engine's "later file wins" doesn't care whether the
	// two files belong to the same mod or different mods.
	candidates := collapseSameMod(raw, rule)

	if len(candidates) == 1 {
		return Resolution{Key: k, Winner: candidates[0], Reason: ReasonSingle}, nil
	}

	if allSameHash(candidates) {
		winner := pickByRule(candidates, order, rule)
		return Resolution{
			Key: k, Winner: winner, Reason: ReasonDuplicate, Rule: rule,
			Losers: withoutWinner(candidates, winner),
		}, nil
	}

	// A Type the engine merges natively (see EngineMergedTypes) is never a
	// real conflict regardless of content, so this is checked before
	// dependency-based suppression even gets computed - the engine doesn't
	// care whether the competing mods declare a dependency on each other,
	// it merges them either way.
	if EngineMergedTypes[k.Type] {
		winner := pickByRule(candidates, order, rule)
		return Resolution{
			Key: k, Winner: winner, Reason: ReasonEngineMerged, Rule: rule,
			Losers: withoutWinner(candidates, winner),
		}, nil
	}

	// A localisation Key with exactly one candidate in Stellaris' own
	// "replace" folder (see replaceFolderCandidates) always wins
	// unconditionally in-game - never a genuine conflict. The rare case of
	// 2+ mods both using the convention for the same Key falls through to
	// ordinary resolution below, scoped to just those candidates: every
	// non-replace-folder candidate is already a guaranteed loser and is
	// dropped from consideration here.
	if restricted, ok := replaceFolderCandidates(k.Type, candidates); ok {
		if len(restricted) == 1 {
			return Resolution{
				Key: k, Winner: restricted[0], Reason: ReasonReplaceFolder,
				Losers: withoutWinner(candidates, restricted[0]),
			}, nil
		}
		candidates = restricted
	}

	ids := modIDs(candidates)
	if deps.fullyConnected(ids) {
		winner := pickByRule(candidates, order, rule)
		return Resolution{
			Key: k, Winner: winner, Reason: ReasonSuppressed, Rule: rule,
			Losers: withoutWinner(candidates, winner), SuppressedBy: suppressingEdge(deps, ids),
		}, nil
	}

	winner := pickByRule(candidates, order, rule)
	res := Resolution{
		Key: k, Winner: winner, Reason: ReasonResolved, Rule: rule,
		Losers: withoutWinner(candidates, winner),
	}
	conflict := &Conflict{Key: k, Candidates: orderByLoadOrder(candidates, order), Rule: rule}
	return res, conflict
}

// collapseSameMod groups raw by ModID (preserving each mod's own original
// relative order - see Index's doc comment) and picks one representative
// per mod per rule: FIOS keeps the first, LIOS (the default) keeps the
// last. The result has exactly one Definition per distinct ModID in raw,
// in first-appearance order.
func collapseSameMod(raw []definition.Definition, rule PriorityRule) []definition.Definition {
	var modOrder []string
	grouped := map[string][]definition.Definition{}
	for _, d := range raw {
		if _, ok := grouped[d.ModID]; !ok {
			modOrder = append(modOrder, d.ModID)
		}
		grouped[d.ModID] = append(grouped[d.ModID], d)
	}

	result := make([]definition.Definition, 0, len(modOrder))
	for _, modID := range modOrder {
		defs := grouped[modID]
		if rule == FIOS {
			result = append(result, defs[0])
		} else {
			result = append(result, defs[len(defs)-1])
		}
	}
	return result
}

func allSameHash(defs []definition.Definition) bool {
	for i := 1; i < len(defs); i++ {
		if defs[i].Hash != defs[0].Hash {
			return false
		}
	}
	return true
}

// pickByRule picks the winner among candidates (already one-per-mod) by
// load-order position: LIOS takes the highest position (latest), FIOS the
// lowest (earliest).
func pickByRule(candidates []definition.Definition, order LoadOrder, rule PriorityRule) definition.Definition {
	best := candidates[0]
	bestPos, _ := order.Priority(best.ModID)
	for _, d := range candidates[1:] {
		pos, ok := order.Priority(d.ModID)
		if !ok {
			continue
		}
		if rule == FIOS {
			if pos < bestPos {
				best, bestPos = d, pos
			}
		} else if pos > bestPos {
			best, bestPos = d, pos
		}
	}
	return best
}

// withoutWinner returns every candidate except the winner's mod. Safe to
// compare by ModID alone since candidates is already one-per-mod (post
// collapseSameMod).
func withoutWinner(candidates []definition.Definition, winner definition.Definition) []definition.Definition {
	var losers []definition.Definition
	for _, d := range candidates {
		if d.ModID != winner.ModID {
			losers = append(losers, d)
		}
	}
	return losers
}

func modIDs(defs []definition.Definition) []string {
	ids := make([]string, len(defs))
	for i, d := range defs {
		ids[i] = d.ModID
	}
	return ids
}

// suppressingEdge returns the first (in ids order, which is deterministic
// - see collapseSameMod) declared dependency edge connecting any pair in
// ids, for the Resolution's audit trail.
func suppressingEdge(deps dependencyGraph, ids []string) *DependencyEdge {
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			if e, ok := deps.edge(ids[i], ids[j]); ok {
				return &e
			}
		}
	}
	return nil
}

// orderByLoadOrder returns a copy of candidates sorted by ascending
// load-order position, for Conflict.Candidates.
func orderByLoadOrder(candidates []definition.Definition, order LoadOrder) []definition.Definition {
	sorted := make([]definition.Definition, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		pi, _ := order.Priority(sorted[i].ModID)
		pj, _ := order.Priority(sorted[j].ModID)
		return pi < pj
	})
	return sorted
}
