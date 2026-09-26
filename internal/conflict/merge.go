package conflict

import (
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
	"github.com/Official-Husko/parallax-mod-manager/internal/xhash"
)

// AdditiveEntries computes which of candidate's own entries are safe to
// splice into winner as new content - Tier 1 of docs/merge-patch.md. It is
// pure: no file I/O, no knowledge of which mod anything came from or which
// Type this is (the caller decides whether a Type is even eligible via
// MergeSafeTypes, and passes the matching RepeatableMergeKeys entry, if
// any, for it). The caller supplies already-parsed Blocks - re-parsed from
// each candidate's own definition.Span, see docs/merge-patch.md's "What the
// codebase already gives it".
//
// Matching, per candidate entry c, in order:
//  1. If some entry in winner has the exact same normalized content (key,
//     operator, and value all identical, via definition.NormalizeEntry), c
//     is already effectively present - not added, not a failure.
//  2. Else, if c has a key and that key is in repeatable, c is a new,
//     distinct instance of a confirmed legitimately-repeating field (e.g.
//     Stellaris' advanced_authority_swap - see
//     docs/merge-safety-by-type.md) - always safe to add regardless of how
//     many entries already share that key.
//  3. Else, if some entry in winner shares c's own key (and, by step 1,
//     therefore has different content), that's a genuine disagreement over
//     the same named thing - ok becomes false and the whole call fails.
//     Tier 1 refuses rather than guess which version is right, matching
//     docs/merge-patch.md's Tier 1 description.
//  4. Else, c's key (or c itself, if it has none - a bare list entry, which
//     is always matched by content alone) doesn't exist in winner at all -
//     a safe, unambiguous addition.
//
// Entries are returned in candidate's own original order. The moment any
// entry hits step 3, ok is false and added is nil - a partial splice is
// never useful, since the caller (the mod-level atomicity in
// docs/merge-patch.md) always discards a mod's whole batch on any single
// failure anyway.
func AdditiveEntries(winner, candidate script.Block, repeatable map[string]bool) (added []script.Entry, ok bool) {
	winnerHashes := make(map[uint64]bool, len(winner.Entries))
	winnerKeys := make(map[string]bool, len(winner.Entries))
	for _, w := range winner.Entries {
		winnerHashes[xhash.Definition(definition.NormalizeEntry(w))] = true
		if w.Key != "" {
			winnerKeys[w.Key] = true
		}
	}

	for _, c := range candidate.Entries {
		if winnerHashes[xhash.Definition(definition.NormalizeEntry(c))] {
			continue // already effectively present, byte-for-byte equivalent
		}
		if c.Key != "" && repeatable[c.Key] {
			added = append(added, c)
			continue
		}
		if c.Key != "" && winnerKeys[c.Key] {
			return nil, false // same identity, different content - a real disagreement
		}
		added = append(added, c)
	}
	return added, true
}
