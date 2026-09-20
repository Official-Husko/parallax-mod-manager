package conflict

import (
	"runtime"
	"sort"
	"sync"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
)

// Index is the cross-mod (Type,ID) picture: every Definition any input mod
// contributes, grouped by Key, restricted to mods present in the LoadOrder
// BuildIndex was given. It does no rule-based collapsing - a single mod
// can appear more than once at the same Key if it defines that object in
// more than one of its own files; see detectKey for where that gets
// resolved.
//
// Within one key's entries, those belonging to the same mod always appear in that
// mod's own original Defs order (BuildIndex iterates each input's Defs in
// order and appends as it goes) - detectKey relies on this to collapse
// same-mod duplicates correctly without needing a separate ordinal field.
type Index struct {
	// shards partition the keys by hash so BuildIndex can fill them in
	// parallel - see BuildIndex. A key always lives in exactly one shard
	// (shardOf), so a lookup only ever needs to look in that one.
	shards []map[Key][]definition.Definition
}

// shardThreshold is how many definitions BuildIndex needs before it shards.
// A variable, not a constant, only so a test can force the parallel path on
// inputs small enough to check exhaustively.
var shardThreshold = 50000

// maxIndexShards caps how many shards (and so goroutines) BuildIndex uses.
// Past this the per-shard work is too small to be worth the extra hashing.
const maxIndexShards = 8

// shardOf returns which of n shards k belongs to: FNV-1a over the key's Type
// and ID, folded and reduced. Deterministic on purpose - a per-index random
// seed (hash/maphash) would make two builds of the same input unequal, and
// nothing about a shard assignment is worth being unpredictable. It's inline
// and allocation-free because BuildIndex calls it once per definition per
// shard.
func shardOf(k Key, n int) int {
	if n == 1 {
		return 0
	}
	const offset, prime = 14695981039346656037, 1099511628211
	h := uint64(offset)
	for i := 0; i < len(k.Type); i++ {
		h = (h ^ uint64(k.Type[i])) * prime
	}
	h = (h ^ 0xff) * prime // separates "ab"+"c" from "a"+"bc"
	for i := 0; i < len(k.ID); i++ {
		h = (h ^ uint64(k.ID[i])) * prime
	}
	h ^= h >> 32
	return int(h % uint64(n))
}

// BuildIndex groups every input's Definitions by Key, dropping any
// Definition whose ModID isn't present in order (a mod that isn't enabled
// can't be launched, so it can't conflict).
//
// On a big modlist this is the single biggest step of conflict detection -
// hundreds of thousands of definitions, each a hash-and-insert into one
// giant map - so it's done in parallel: the keys are split by hash into
// shards, and one goroutine per shard walks *every* input in order, keeping
// only the definitions that hash to its shard. Each shard therefore sees its
// definitions in exactly the order a single sequential pass would have, which
// is what the same-mod ordering guarantee above relies on, and no shard's
// map is ever touched by two goroutines.
func BuildIndex(order LoadOrder, inputs []Input) Index {
	var enabled []Input
	total := 0
	for _, inp := range inputs {
		if _, ok := order.Priority(inp.Mod.ID); ok {
			enabled = append(enabled, inp)
			total += len(inp.Defs)
		}
	}

	// Small inputs aren't worth the goroutines and the redundant hashing.
	n := 1
	if total >= shardThreshold {
		n = min(runtime.NumCPU(), maxIndexShards)
	}
	ix := Index{shards: make([]map[Key][]definition.Definition, n)}

	fill := func(shard int) {
		// Sized up front: growing a map this large one doubling at a time
		// re-hashes and re-copies every entry each time. Most definitions are
		// a key of their own, so a share of the total is a good first guess.
		m := make(map[Key][]definition.Definition, total/n+1)
		for _, inp := range enabled {
			for _, d := range inp.Defs {
				k := Key{Type: d.Type, ID: d.ID}
				if shardOf(k, n) == shard {
					m[k] = append(m[k], d)
				}
			}
		}
		ix.shards[shard] = m
	}
	if n == 1 {
		fill(0)
		return ix
	}
	var wg sync.WaitGroup
	for shard := 0; shard < n; shard++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fill(shard)
		}()
	}
	wg.Wait()
	return ix
}

// At returns every mod's raw Definition(s) at k. ok is false if no input
// mod touches k at all.
func (ix Index) At(k Key) (defs []definition.Definition, ok bool) {
	if len(ix.shards) == 0 {
		return nil, false
	}
	defs, ok = ix.shards[shardOf(k, len(ix.shards))][k]
	return defs, ok
}

// each calls fn for every key in the index, in no particular order.
func (ix Index) each(fn func(k Key, defs []definition.Definition)) {
	for _, m := range ix.shards {
		for k, defs := range m {
			fn(k, defs)
		}
	}
}

// size is how many distinct keys the index holds.
func (ix Index) size() int {
	n := 0
	for _, m := range ix.shards {
		n += len(m)
	}
	return n
}

// Keys returns every Key touched by 2+ distinct mods - the candidate set a
// caller walking for genuine conflicts needs - sorted by (Type, ID) for
// deterministic iteration (map iteration order is not stable).
func (ix Index) Keys() []Key {
	var keys []Key
	ix.each(func(k Key, defs []definition.Definition) {
		if distinctModCount(defs) > 1 {
			keys = append(keys, k)
		}
	})
	sort.Slice(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	return keys
}

// allKeys returns every Key touched by any mod at all, sorted - used
// internally by Resolve, which needs a Resolution for every Key, not just
// contested ones.
func (ix Index) allKeys() []Key {
	keys := make([]Key, 0, ix.size())
	ix.each(func(k Key, _ []definition.Definition) {
		keys = append(keys, k)
	})
	sort.Slice(keys, func(i, j int) bool { return keyLess(keys[i], keys[j]) })
	return keys
}

// singleMod reports whether every definition in defs belongs to one mod, which
// (after collapsing that mod's own duplicates) means the key cannot conflict.
// It allocates nothing, unlike distinctModCount.
func singleMod(defs []definition.Definition) bool {
	for i := 1; i < len(defs); i++ {
		if defs[i].ModID != defs[0].ModID {
			return false
		}
	}
	return true
}

func distinctModCount(defs []definition.Definition) int {
	seen := map[string]struct{}{}
	for _, d := range defs {
		seen[d.ModID] = struct{}{}
	}
	return len(seen)
}
