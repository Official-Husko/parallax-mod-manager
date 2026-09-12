package conflict

import (
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
