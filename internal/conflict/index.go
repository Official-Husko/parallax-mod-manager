package conflict

import (
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// Index is the cross-mod (Type,ID) picture: every Definition any input mod
// contributes, grouped by Key, restricted to mods present in the LoadOrder
// BuildIndex was given. It does no rule-based collapsing - a single mod
// can appear more than once at the same Key if it defines that object in
// more than one of its own files; see detectKey for where that gets
// resolved.
//
// Within byKey[k], entries belonging to the same mod always appear in that
// mod's own original Defs order (BuildIndex iterates each input's Defs in
// order and appends as it goes) - detectKey relies on this to collapse
// same-mod duplicates correctly without needing a separate ordinal field.
type Index struct {
	byKey map[Key][]definition.Definition
}

// BuildIndex groups every input's Definitions by Key, dropping any
// Definition whose ModID isn't present in order (a mod that isn't enabled
// can't be launched, so it can't conflict).
func BuildIndex(order LoadOrder, inputs []Input) Index {
	byKey := map[Key][]definition.Definition{}
	for _, inp := range inputs {
		if _, ok := order.Priority(inp.Mod.ID); !ok {
			continue
		}
		for _, d := range inp.Defs {
			k := Key{Type: d.Type, ID: d.ID}
			byKey[k] = append(byKey[k], d)
		}
	}
	return Index{byKey: byKey}
}

// At returns every mod's raw Definition(s) at k. ok is false if no input
// mod touches k at all.
func (ix Index) At(k Key) (defs []definition.Definition, ok bool) {
	defs, ok = ix.byKey[k]
	return defs, ok
}

// Keys returns every Key touched by 2+ distinct mods - the candidate set a
// caller walking for genuine conflicts needs - sorted by (Type, ID) for
// deterministic iteration (map iteration order is not stable).
func (ix Index) Keys() []Key {
	var keys []Key
	for k, defs := range ix.byKey {
		if distinctModCount(defs) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	return keys
}

// allKeys returns every Key touched by any mod at all, sorted - used
// internally by Resolve, which needs a Resolution for every Key, not just
// contested ones.
func (ix Index) allKeys() []Key {
	keys := make([]Key, 0, len(ix.byKey))
	for k := range ix.byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	return keys
}

func distinctModCount(defs []definition.Definition) int {
	seen := map[string]struct{}{}
	for _, d := range defs {
		seen[d.ModID] = struct{}{}
	}
	return len(seen)
}
