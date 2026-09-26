package conflict

import (
	"reflect"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

func TestLoadOrderPriority(t *testing.T) {
	order := LoadOrder{"mod_a", "mod_b", "mod_c"}

	tests := []struct {
		modID   string
		wantPos int
		wantOK  bool
	}{
		{"mod_a", 0, true},
		{"mod_b", 1, true},
		{"mod_c", 2, true},
		{"mod_missing", 0, false},
	}
	for _, tt := range tests {
		pos, ok := order.Priority(tt.modID)
		if pos != tt.wantPos || ok != tt.wantOK {
			t.Errorf("Priority(%q) = (%d, %v), want (%d, %v)", tt.modID, pos, ok, tt.wantPos, tt.wantOK)
		}
	}
}

func TestLoadOrderPriorityDuplicateIDUsesFirstOccurrence(t *testing.T) {
	order := LoadOrder{"mod_a", "mod_b", "mod_a"}
	pos, ok := order.Priority("mod_a")
	if !ok || pos != 0 {
		t.Errorf("Priority(\"mod_a\") = (%d, %v), want (0, true) - first occurrence", pos, ok)
	}
}

func TestPriorityRulesRuleFor(t *testing.T) {
	rules := PriorityRules{"scripted_variables": FIOS}

	if got := rules.RuleFor("scripted_variables"); got != FIOS {
		t.Errorf("RuleFor(explicit FIOS entry) = %v, want FIOS", got)
	}
	if got := rules.RuleFor("common/buildings"); got != LIOS {
		t.Errorf("RuleFor(absent type) = %v, want LIOS (default)", got)
	}
}

func TestPriorityRulesRuleForNilMapDefaultsToLIOS(t *testing.T) {
	var rules PriorityRules // nil
	if got := rules.RuleFor("anything"); got != LIOS {
		t.Errorf("RuleFor on nil PriorityRules = %v, want LIOS", got)
	}
}

func TestDefaultPriorityRulesIsNearEmpty(t *testing.T) {
	// Regression guard: don't let someone "helpfully" add unverified
	// Paradox Type names without a confirmed source - see
	// docs/conflict-resolution.md and this var's doc comment. Exactly six
	// entries are confirmed so far (see the doc comment for each one's
	// source) - this pins the map to precisely those, so a new entry still
	// has to be a deliberate, reviewed addition rather than something that
	// crept in unnoticed.
	want := PriorityRules{
		"common/static_modifiers":    FIOS,
		"common/component_sets":      FIOS,
		"common/component_templates": FIOS,
		"common/global_ship_designs": FIOS,
		"common/scripted_loc":        FIOS,
		"common/scripted_variables":  FIOS,
	}
	if len(DefaultPriorityRules) != len(want) {
		t.Fatalf("DefaultPriorityRules has %d entries, want exactly %d (see its doc comment before adding any more)", len(DefaultPriorityRules), len(want))
	}
	for typ, rule := range want {
		if got := DefaultPriorityRules[typ]; got != rule {
			t.Errorf("DefaultPriorityRules[%q] = %v, want %v", typ, got, rule)
		}
	}
}

func TestEngineMergedTypesIsNearEmpty(t *testing.T) {
	// Same regression guard as TestDefaultPriorityRulesIsNearEmpty, for the
	// same reason - see EngineMergedTypes' own doc comment. Exactly one
	// entry is confirmed so far.
	want := map[definition.Type]bool{"common/on_actions": true}
	if len(EngineMergedTypes) != len(want) {
		t.Fatalf("EngineMergedTypes has %d entries, want exactly %d (see its doc comment before adding any more)", len(EngineMergedTypes), len(want))
	}
	for typ, v := range want {
		if got := EngineMergedTypes[typ]; got != v {
			t.Errorf("EngineMergedTypes[%q] = %v, want %v", typ, got, v)
		}
	}
}

func TestMergeSafeTypesIsNearEmpty(t *testing.T) {
	// Same regression guard as TestDefaultPriorityRulesIsNearEmpty, for the
	// same reason - see MergeSafeTypes' own doc comment. Exactly one entry
	// is confirmed so far.
	want := map[definition.Type]bool{"common/governments/authorities": true}
	if len(MergeSafeTypes) != len(want) {
		t.Fatalf("MergeSafeTypes has %d entries, want exactly %d (see its doc comment before adding any more)", len(MergeSafeTypes), len(want))
	}
	for typ, v := range want {
		if got := MergeSafeTypes[typ]; got != v {
			t.Errorf("MergeSafeTypes[%q] = %v, want %v", typ, got, v)
		}
	}
}

func TestRepeatableMergeKeysIsNearEmpty(t *testing.T) {
	// Same regression guard again - see RepeatableMergeKeys' own doc comment.
	want := map[definition.Type]map[string]bool{
		"common/governments/authorities": {"advanced_authority_swap": true},
	}
	if len(RepeatableMergeKeys) != len(want) {
		t.Fatalf("RepeatableMergeKeys has %d entries, want exactly %d (see its doc comment before adding any more)", len(RepeatableMergeKeys), len(want))
	}
	for typ, keys := range want {
		got := RepeatableMergeKeys[typ]
		if len(got) != len(keys) {
			t.Fatalf("RepeatableMergeKeys[%q] has %d entries, want exactly %d", typ, len(got), len(keys))
		}
		for key, v := range keys {
			if got[key] != v {
				t.Errorf("RepeatableMergeKeys[%q][%q] = %v, want %v", typ, key, got[key], v)
			}
		}
	}
}

// --- Resolve integration tests -------------------------------------------

func mustDefs(t *testing.T, modID, relPath, defType, src string) []definition.Definition {
	t.Helper()
	f, err := script.Parse([]byte(src))
	if err != nil {
		t.Fatalf("script.Parse: %v", err)
	}
	return definition.FromScriptFile(modID, relPath, definition.Type(defType), f)
}

func TestResolveEndToEndThreeMods(t *testing.T) {
	// mod_base defines a building; mod_a and mod_b both override it
	// differently and don't declare any dependency on each other, so it's
	// a genuine conflict resolved by load order (LIOS).
	base := mustDefs(t, "mod_base", "common/buildings/x.txt", "common/buildings", `some_building = { cost = 100 }`)
	a := mustDefs(t, "mod_a", "common/buildings/x.txt", "common/buildings", `some_building = { cost = 200 }`)
	b := mustDefs(t, "mod_b", "common/buildings/x.txt", "common/buildings", `some_building = { cost = 300 }`)
	// mod_c only touches an unrelated object - should resolve as
	// ReasonSingle, not appear in Conflicts.
	c := mustDefs(t, "mod_c", "events/y.txt", "events", `some_event = { id = e.1 }`)

	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_base"}, Defs: base},
		{Mod: mod.Mod{ID: "mod_a"}, Defs: a},
		{Mod: mod.Mod{ID: "mod_b"}, Defs: b},
		{Mod: mod.Mod{ID: "mod_c"}, Defs: c},
	}
	order := LoadOrder{"mod_base", "mod_a", "mod_b", "mod_c"}

	result := Resolve(order, inputs, Options{})

	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(result.Conflicts), result.Conflicts)
	}
	buildingKey := Key{Type: "common/buildings", ID: "some_building"}
	conflict := result.Conflicts[0]
	if conflict.Key != buildingKey {
		t.Errorf("Conflict.Key = %+v, want %+v", conflict.Key, buildingKey)
	}
	if len(conflict.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(conflict.Candidates))
	}
	// Ascending load-order position: mod_base, mod_a, mod_b.
	wantOrder := []string{"mod_base", "mod_a", "mod_b"}
	for i, d := range conflict.Candidates {
		if d.ModID != wantOrder[i] {
			t.Errorf("Candidates[%d].ModID = %q, want %q", i, d.ModID, wantOrder[i])
		}
	}

	buildingRes, ok := result.Resolutions[buildingKey]
	if !ok {
		t.Fatal("missing resolution for building key")
	}
	if buildingRes.Reason != ReasonResolved || buildingRes.Winner.ModID != "mod_b" {
		t.Errorf("building resolution = %+v, want Reason=ReasonResolved Winner.ModID=mod_b (last in load order)", buildingRes)
	}

	eventKey := Key{Type: "events", ID: "some_event"}
	eventRes, ok := result.Resolutions[eventKey]
	if !ok {
		t.Fatal("missing resolution for event key")
	}
	if eventRes.Reason != ReasonSingle || eventRes.Winner.ModID != "mod_c" {
		t.Errorf("event resolution = %+v, want ReasonSingle/mod_c", eventRes)
	}
}

func TestResolveDependencySuppression(t *testing.T) {
	base := mustDefs(t, "mod_base", "common/x.txt", "common", `thing = { a = 1 }`)
	patch := mustDefs(t, "mod_patch", "common/x.txt", "common", `thing = { a = 2 }`)

	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_base", Descriptor: mod.Descriptor{Name: "Base Mod"}}, Defs: base},
		{
			Mod: mod.Mod{
				ID:         "mod_patch",
				Descriptor: mod.Descriptor{Name: "Patch For Base", Dependencies: []string{"Base Mod"}},
			},
			Defs: patch,
		},
	}
	order := LoadOrder{"mod_base", "mod_patch"}

	result := Resolve(order, inputs, Options{})

	if len(result.Conflicts) != 0 {
		t.Fatalf("expected the dependency-declared override to be suppressed, got conflicts: %+v", result.Conflicts)
	}
	key := Key{Type: "common", ID: "thing"}
	res, ok := result.Resolutions[key]
	if !ok {
		t.Fatal("missing resolution")
	}
	if res.Reason != ReasonSuppressed {
		t.Errorf("Reason = %v, want ReasonSuppressed", res.Reason)
	}
	if res.Winner.ModID != "mod_patch" {
		t.Errorf("Winner.ModID = %q, want mod_patch (still chosen by load order, not dependency direction)", res.Winner.ModID)
	}
	if res.SuppressedBy == nil {
		t.Fatal("expected SuppressedBy to be populated")
	}
}

func TestResolveCustomRulesOverrideDefault(t *testing.T) {
	a := mustDefs(t, "mod_a", "common/x.txt", "test/fios-type", `thing = { a = 1 }`)
	b := mustDefs(t, "mod_b", "common/x.txt", "test/fios-type", `thing = { a = 2 }`)

	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_a"}, Defs: a},
		{Mod: mod.Mod{ID: "mod_b"}, Defs: b},
	}
	order := LoadOrder{"mod_a", "mod_b"}

	result := Resolve(order, inputs, Options{Rules: PriorityRules{"test/fios-type": FIOS}})

	key := Key{Type: "test/fios-type", ID: "thing"}
	res := result.Resolutions[key]
	if res.Winner.ModID != "mod_a" {
		t.Errorf("under a custom FIOS rule, Winner.ModID = %q, want mod_a (earliest in load order)", res.Winner.ModID)
	}
}

func TestResolveDeterministicAcrossRuns(t *testing.T) {
	base := mustDefs(t, "mod_base", "common/x.txt", "common", `thing = { a = 1 }`)
	a := mustDefs(t, "mod_a", "common/x.txt", "common", `thing = { a = 2 }`)
	b := mustDefs(t, "mod_b", "common/x.txt", "common", `thing = { a = 3 }`)

	inputs := []Input{
		{Mod: mod.Mod{ID: "mod_base"}, Defs: base},
		{Mod: mod.Mod{ID: "mod_a"}, Defs: a},
		{Mod: mod.Mod{ID: "mod_b"}, Defs: b},
	}
	order := LoadOrder{"mod_base", "mod_a", "mod_b"}

	first := Resolve(order, inputs, Options{})
	for i := 0; i < 10; i++ {
		got := Resolve(order, inputs, Options{})
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d produced a different Result:\n got  %+v\n want %+v", i, got, first)
		}
	}
}

func TestResolveEmptyInputs(t *testing.T) {
	result := Resolve(LoadOrder{}, nil, Options{})
	if len(result.Conflicts) != 0 || len(result.Resolutions) != 0 {
		t.Errorf("expected an empty Result, got %+v", result)
	}
}

// randomInputs builds a reproducible pile of mods whose definitions overlap
// heavily: same-mod duplicates, identical-hash duplicates across mods, real
// conflicts, and declared dependencies that suppress some of them - every
// branch detectKey has.
func randomInputs(seed uint64, mods, keys int) ([]Input, LoadOrder) {
	next := func() uint64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return seed >> 33
	}
	inputs := make([]Input, mods)
	order := make(LoadOrder, mods)
	for i := range inputs {
		id := "mod_" + string(rune('a'+i))
		order[i] = id
		var deps []string
		for j := 0; j < mods; j++ {
			if j != i && next()%6 == 0 {
				deps = append(deps, "Mod "+string(rune('a'+j)))
			}
		}
		inputs[i].Mod = mod.Mod{ID: id, Descriptor: mod.Descriptor{Name: "Mod " + string(rune('a'+i)), Dependencies: deps}}
		for n := uint64(0); n < uint64(keys); n++ {
			if next()%3 == 0 {
				continue
			}
			key := "key_" + string(rune('a'+next()%uint64(keys)%26)) + string(rune('a'+next()%7))
			typ := definition.Type("common/thing_" + string(rune('a'+next()%3)))
			// A small hash alphabet makes identical-content duplicates common.
			inputs[i].Defs = append(inputs[i].Defs, definition.Definition{
				Type: typ, ID: key, ModID: id, FilePath: "f" + string(rune('a'+next()%3)) + ".txt", Hash: next() % 3, Order: int(n),
			})
		}
	}
	return inputs, order
}

func TestConflictsOnlyGivesTheSameConflictsAsTheFullResolve(t *testing.T) {
	checked, withConflicts := 0, 0
	for seed := uint64(1); seed <= 200; seed++ {
		inputs, order := randomInputs(seed, 2+int(seed%7), 5+int(seed%30))
		full := Resolve(order, inputs, Options{})
		fast := Resolve(order, inputs, Options{ConflictsOnly: true})
		checked++
		if len(full.Conflicts) > 0 {
			withConflicts++
		}
		if !reflect.DeepEqual(full.Conflicts, fast.Conflicts) {
			t.Fatalf("seed %d: conflicts differ\nfull: %+v\nfast: %+v", seed, full.Conflicts, fast.Conflicts)
		}
		if fast.Resolutions != nil {
			t.Fatalf("seed %d: ConflictsOnly must not build Resolutions", seed)
		}
	}
	if withConflicts < 50 {
		t.Errorf("only %d of %d random cases had a conflict - the generator isn't exercising the interesting branches", withConflicts, checked)
	}
}

func TestConflictsOnlyHonoursPriorityRules(t *testing.T) {
	inputs, order := randomInputs(7, 5, 25)
	rules := PriorityRules{"common/thing_a": FIOS, "common/thing_b": FIOS}
	full := Resolve(order, inputs, Options{Rules: rules})
	fast := Resolve(order, inputs, Options{Rules: rules, ConflictsOnly: true})
	if !reflect.DeepEqual(full.Conflicts, fast.Conflicts) {
		t.Errorf("first-in-wins types produced different conflicts")
	}
}

func TestSingleModDetection(t *testing.T) {
	d := func(mod string) definition.Definition { return definition.Definition{ModID: mod} }
	if !singleMod(nil) || !singleMod([]definition.Definition{d("a")}) || !singleMod([]definition.Definition{d("a"), d("a")}) {
		t.Error("zero, one, or repeated same-mod definitions can't conflict")
	}
	if singleMod([]definition.Definition{d("a"), d("b")}) || singleMod([]definition.Definition{d("a"), d("a"), d("b")}) {
		t.Error("two different mods can conflict")
	}
}
