package conflict

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func def(modID, defType, id string, hash uint64) definition.Definition {
	return definition.Definition{Type: definition.Type(defType), ID: id, ModID: modID, Hash: hash}
}

func TestBuildIndexGroupsByKey(t *testing.T) {
	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_a"}, Defs: []definition.Definition{def("mod_a", "common", "thing", 1)}},
		{Mod: mod.Mod{ID: "mod_b"}, Defs: []definition.Definition{def("mod_b", "common", "thing", 2)}},
	}
	ix := BuildIndex(LoadOrder{"mod_a", "mod_b"}, inputs)

	defs, ok := ix.At(Key{Type: "common", ID: "thing"})
	if !ok || len(defs) != 2 {
		t.Fatalf("At() = %+v, ok=%v, want 2 entries", defs, ok)
	}
}

func TestBuildIndexExcludesModsNotInLoadOrder(t *testing.T) {
	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_a"}, Defs: []definition.Definition{def("mod_a", "common", "thing", 1)}},
		{Mod: mod.Mod{ID: "mod_disabled"}, Defs: []definition.Definition{def("mod_disabled", "common", "thing", 2)}},
	}
	// mod_disabled is not in the load order.
	ix := BuildIndex(LoadOrder{"mod_a"}, inputs)

	defs, ok := ix.At(Key{Type: "common", ID: "thing"})
	if !ok || len(defs) != 1 || defs[0].ModID != "mod_a" {
		t.Fatalf("At() = %+v, ok=%v, want exactly mod_a's definition", defs, ok)
	}
}

func TestIndexAtUnknownKey(t *testing.T) {
	ix := BuildIndex(LoadOrder{}, nil)
	if _, ok := ix.At(Key{Type: "common", ID: "nope"}); ok {
		t.Error("expected ok=false for a key nothing touches")
	}
}

func TestIndexKeysOnlyReturnsMultiModKeys(t *testing.T) {
	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_a"}, Defs: []definition.Definition{
			def("mod_a", "common", "shared", 1),
			def("mod_a", "common", "solo", 5),
		}},
		{Mod: mod.Mod{ID: "mod_b"}, Defs: []definition.Definition{
			def("mod_b", "common", "shared", 2),
		}},
	}
	ix := BuildIndex(LoadOrder{"mod_a", "mod_b"}, inputs)

	keys := ix.Keys()
	if len(keys) != 1 || keys[0] != (Key{Type: "common", ID: "shared"}) {
		t.Errorf("Keys() = %+v, want exactly [{common shared}]", keys)
	}
}

func TestIndexKeysSortedDeterministically(t *testing.T) {
	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_a"}, Defs: []definition.Definition{
			def("mod_a", "zzz", "id1", 1),
			def("mod_a", "aaa", "id2", 1),
			def("mod_a", "aaa", "id1", 1),
		}},
		{Mod: mod.Mod{ID: "mod_b"}, Defs: []definition.Definition{
			def("mod_b", "zzz", "id1", 2),
			def("mod_b", "aaa", "id2", 2),
			def("mod_b", "aaa", "id1", 2),
		}},
	}
	ix := BuildIndex(LoadOrder{"mod_a", "mod_b"}, inputs)

	want := []Key{
		{Type: "aaa", ID: "id1"},
		{Type: "aaa", ID: "id2"},
		{Type: "zzz", ID: "id1"},
	}
	got := ix.Keys()
	if len(got) != len(want) {
		t.Fatalf("Keys() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Keys()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestIndexAtPreservesSameModOrder(t *testing.T) {
	// Same mod, same key, two different files (e.g. an override file) -
	// Index must not collapse or reorder these; detectKey relies on
	// original order being preserved to apply LIOS/FIOS correctly.
	first := def("mod_a", "common", "thing", 1)
	first.FilePath = "common/00_thing.txt"
	second := def("mod_a", "common", "thing", 2)
	second.FilePath = "common/99_thing_override.txt"

	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_a"}, Defs: []definition.Definition{first, second}},
	}
	ix := BuildIndex(LoadOrder{"mod_a"}, inputs)

	defs, ok := ix.At(Key{Type: "common", ID: "thing"})
	if !ok || len(defs) != 2 {
		t.Fatalf("At() = %+v, ok=%v, want 2 entries", defs, ok)
	}
	if defs[0].FilePath != first.FilePath || defs[1].FilePath != second.FilePath {
		t.Errorf("At() did not preserve original order: %+v", defs)
	}
}

// withShardThreshold runs fn with BuildIndex's parallel threshold changed.
func withShardThreshold(t *testing.T, v int, fn func()) {
	t.Helper()
	old := shardThreshold
	shardThreshold = v
	defer func() { shardThreshold = old }()
	fn()
}

func TestShardedIndexIsIdenticalToTheSequentialOne(t *testing.T) {
	for seed := uint64(1); seed <= 60; seed++ {
		inputs, order := randomInputs(seed, 2+int(seed%7), 8+int(seed%40))

		var seq, par Index
		withShardThreshold(t, 1<<30, func() { seq = BuildIndex(order, inputs) })
		withShardThreshold(t, 0, func() { par = BuildIndex(order, inputs) })

		if len(seq.shards) != 1 {
			t.Fatalf("seed %d: expected the sequential index to use one shard, got %d", seed, len(seq.shards))
		}
		if seed == 1 && runtime.NumCPU() > 1 && len(par.shards) < 2 {
			t.Fatalf("expected the forced parallel index to use several shards, got %d", len(par.shards))
		}
		if seq.size() != par.size() {
			t.Fatalf("seed %d: %d keys sequentially, %d sharded", seed, seq.size(), par.size())
		}
		seq.each(func(k Key, want []definition.Definition) {
			got, ok := par.At(k)
			if !ok || !reflect.DeepEqual(got, want) {
				t.Fatalf("seed %d: key %v differs\nsequential: %+v\nsharded:    %+v", seed, k, want, got)
			}
		})
		if _, ok := par.At(Key{Type: "no/such", ID: "key"}); ok {
			t.Fatalf("seed %d: a key nobody defines was found", seed)
		}
	}
}

func TestResolveGivesTheSameResultShardedOrNot(t *testing.T) {
	for seed := uint64(100); seed < 160; seed++ {
		inputs, order := randomInputs(seed, 3+int(seed%5), 10+int(seed%30))
		var seq, par Result
		var seqFast, parFast Result
		withShardThreshold(t, 1<<30, func() {
			seq = Resolve(order, inputs, Options{})
			seqFast = Resolve(order, inputs, Options{ConflictsOnly: true})
		})
		withShardThreshold(t, 0, func() {
			par = Resolve(order, inputs, Options{})
			parFast = Resolve(order, inputs, Options{ConflictsOnly: true})
		})
		if !reflect.DeepEqual(seq.Conflicts, par.Conflicts) || !reflect.DeepEqual(seq.Resolutions, par.Resolutions) {
			t.Fatalf("seed %d: the full resolve differs when the index is sharded", seed)
		}
		if !reflect.DeepEqual(seqFast.Conflicts, parFast.Conflicts) || !reflect.DeepEqual(seq.Conflicts, parFast.Conflicts) {
			t.Fatalf("seed %d: the conflicts-only resolve differs when the index is sharded", seed)
		}
	}
}

func TestIndexKeysAreSortedAndCompleteAcrossShards(t *testing.T) {
	inputs, order := randomInputs(9, 6, 30)
	var par Index
	withShardThreshold(t, 0, func() { par = BuildIndex(order, inputs) })
	all := par.allKeys()
	if len(all) != par.size() {
		t.Errorf("allKeys returned %d keys, index has %d", len(all), par.size())
	}
	for i := 1; i < len(all); i++ {
		if !keyLess(all[i-1], all[i]) {
			t.Fatalf("allKeys isn't strictly sorted at %d: %v then %v", i, all[i-1], all[i])
		}
	}
	for _, k := range par.Keys() {
		defs, _ := par.At(k)
		if distinctModCount(defs) < 2 {
			t.Errorf("Keys returned %v, which only one mod defines", k)
		}
	}
}

func TestShardAssignmentIsDeterministicAndSpreadsKeys(t *testing.T) {
	const n = 8
	counts := make([]int, n)
	for i := 0; i < 8000; i++ {
		k := Key{Type: definition.Type("common/thing_" + string(rune('a'+i%5))), ID: "key_" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26))}
		s := shardOf(k, n)
		if s < 0 || s >= n || s != shardOf(k, n) {
			t.Fatalf("shardOf(%v) = %d, want a stable value in [0,%d)", k, s, n)
		}
		counts[s]++
	}
	for shard, c := range counts {
		if c < 500 || c > 1500 {
			t.Errorf("shard %d got %d of 8000 keys - the split is badly lopsided: %v", shard, c, counts)
		}
	}
	if shardOf(Key{Type: "ab", ID: "c"}, 1000003) == shardOf(Key{Type: "a", ID: "bc"}, 1000003) {
		t.Error("the Type/ID boundary must be part of the hash")
	}
}
