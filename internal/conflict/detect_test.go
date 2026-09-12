package conflict

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

// hashOf produces a real xhash.Definition-derived hash for a script value,
// via the same real parser path definition_test.go's xhashOf helper uses -
// not hand-picked numbers - so tests exercise real hash equality/inequality.
func hashOf(t *testing.T, src string) uint64 {
	t.Helper()
	f, err := script.Parse([]byte(src))
	if err != nil {
		t.Fatalf("script.Parse: %v", err)
	}
	defs := definition.FromScriptFile("m", "f.txt", "t", f)
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 top-level entry, got %d", len(defs))
	}
	return defs[0].Hash
}

func TestDetectKeySingleMod(t *testing.T) {
	raw := []definition.Definition{def("mod_a", "common", "thing", hashOf(t, `thing = { a = 1 }`))}
	res, conflict := detectKey(Key{Type: "common", ID: "thing"}, raw, dependencyGraph{}, LoadOrder{"mod_a"}, nil)

	if res.Reason != ReasonSingle {
		t.Errorf("Reason = %v, want ReasonSingle", res.Reason)
	}
	if res.Winner.ModID != "mod_a" {
		t.Errorf("Winner.ModID = %q, want mod_a", res.Winner.ModID)
	}
	if len(res.Losers) != 0 {
		t.Errorf("Losers = %+v, want none", res.Losers)
	}
	if conflict != nil {
		t.Errorf("expected no Conflict for a single-mod key, got %+v", conflict)
	}
}

func TestDetectKeyDuplicateIdenticalContent(t *testing.T) {
	h := hashOf(t, `thing = { a = 1 }`)
	raw := []definition.Definition{
		def("mod_a", "common", "thing", h),
		def("mod_b", "common", "thing", h),
	}
	res, conflict := detectKey(Key{Type: "common", ID: "thing"}, raw, dependencyGraph{}, LoadOrder{"mod_a", "mod_b"}, nil)

	if res.Reason != ReasonDuplicate {
		t.Errorf("Reason = %v, want ReasonDuplicate", res.Reason)
	}
	if res.Winner.ModID != "mod_b" {
		t.Errorf("Winner.ModID = %q, want mod_b (still resolved deterministically by load order, not slice order)", res.Winner.ModID)
	}
	if conflict != nil {
		t.Errorf("a duplicate must not be reported as a Conflict, got %+v", conflict)
	}
}

func TestDetectKeyGenuineConflictDefaultLIOS(t *testing.T) {
	raw := []definition.Definition{
		def("mod_a", "common", "thing", hashOf(t, `thing = { a = 1 }`)),
		def("mod_b", "common", "thing", hashOf(t, `thing = { a = 2 }`)),
	}
	res, conflict := detectKey(Key{Type: "common", ID: "thing"}, raw, dependencyGraph{}, LoadOrder{"mod_a", "mod_b"}, nil)

	if res.Reason != ReasonResolved {
		t.Errorf("Reason = %v, want ReasonResolved", res.Reason)
	}
	if res.Winner.ModID != "mod_b" {
		t.Errorf("Winner.ModID = %q, want mod_b (LIOS: latest in load order)", res.Winner.ModID)
	}
	if conflict == nil {
		t.Fatal("expected a genuine conflict to be reported")
	}
	if len(conflict.Candidates) != 2 {
		t.Errorf("Candidates = %+v, want 2", conflict.Candidates)
	}
	if len(res.Losers) != 1 || res.Losers[0].ModID != "mod_a" {
		t.Errorf("Losers = %+v, want exactly mod_a", res.Losers)
	}
}

func TestDetectKeyGenuineConflictCustomFIOSRule(t *testing.T) {
	raw := []definition.Definition{
		def("mod_a", "test/fios-type", "thing", hashOf(t, `thing = { a = 1 }`)),
		def("mod_b", "test/fios-type", "thing", hashOf(t, `thing = { a = 2 }`)),
	}
	rules := PriorityRules{"test/fios-type": FIOS}
	res, conflict := detectKey(Key{Type: "test/fios-type", ID: "thing"}, raw, dependencyGraph{}, LoadOrder{"mod_a", "mod_b"}, rules)

	if res.Winner.ModID != "mod_a" {
		t.Errorf("Winner.ModID = %q, want mod_a (FIOS: earliest in load order)", res.Winner.ModID)
	}
	if conflict == nil {
		t.Fatal("expected a genuine conflict to be reported")
	}
	if res.Rule != FIOS {
		t.Errorf("Rule = %v, want FIOS", res.Rule)
	}
}

func TestDetectKeySuppressedByDependency(t *testing.T) {
	raw := []definition.Definition{
		def("mod_base", "common", "thing", hashOf(t, `thing = { a = 1 }`)),
		def("mod_patch", "common", "thing", hashOf(t, `thing = { a = 2 }`)),
	}
	deps := dependencyGraph{}
	deps.addEdge("mod_patch", "mod_base")

	res, conflict := detectKey(Key{Type: "common", ID: "thing"}, raw, deps, LoadOrder{"mod_base", "mod_patch"}, nil)

	if res.Reason != ReasonSuppressed {
		t.Errorf("Reason = %v, want ReasonSuppressed", res.Reason)
	}
	if conflict != nil {
		t.Errorf("a suppressed conflict must not appear as a Conflict, got %+v", conflict)
	}
	if res.SuppressedBy == nil {
		t.Fatal("expected SuppressedBy to be set")
	}
	if res.SuppressedBy.FromModID != "mod_patch" || res.SuppressedBy.ToModID != "mod_base" {
		t.Errorf("SuppressedBy = %+v, want FromModID=mod_patch ToModID=mod_base", res.SuppressedBy)
	}
	// Winner is still chosen by load order, not dependency direction.
	if res.Winner.ModID != "mod_patch" {
		t.Errorf("Winner.ModID = %q, want mod_patch (last in load order)", res.Winner.ModID)
	}
}

func TestDetectKeyThreeWayConflictNotFullySuppressed(t *testing.T) {
	// mod_a<->mod_b connected, but mod_c isn't connected to either - must
	// still surface as a genuine conflict (see fullyConnected's doc
	// comment and docs/conflict-resolution.md).
	raw := []definition.Definition{
		def("mod_a", "common", "thing", hashOf(t, `thing = { a = 1 }`)),
		def("mod_b", "common", "thing", hashOf(t, `thing = { a = 2 }`)),
		def("mod_c", "common", "thing", hashOf(t, `thing = { a = 3 }`)),
	}
	deps := dependencyGraph{}
	deps.addEdge("mod_a", "mod_b")

	res, conflict := detectKey(Key{Type: "common", ID: "thing"}, raw, deps, LoadOrder{"mod_a", "mod_b", "mod_c"}, nil)

	if res.Reason != ReasonResolved {
		t.Errorf("Reason = %v, want ReasonResolved (partial dependency coverage must not suppress)", res.Reason)
	}
	if conflict == nil {
		t.Fatal("expected the partially-explained conflict to still be reported")
	}
	if len(conflict.Candidates) != 3 {
		t.Errorf("Candidates = %+v, want 3", conflict.Candidates)
	}
}

func TestDetectKeySameModCrossFileDuplicateCollapsedByRule(t *testing.T) {
	// One mod, two files defining the same key differently - must
	// collapse to a single candidate before any cross-mod logic runs, and
	// the choice of which file "wins" must respect the Type's rule, just
	// like a cross-mod conflict would.
	early := def("mod_a", "common", "thing", hashOf(t, `thing = { a = 1 }`))
	early.FilePath = "common/00_thing.txt"
	late := def("mod_a", "common", "thing", hashOf(t, `thing = { a = 2 }`))
	late.FilePath = "common/99_thing_override.txt"
	raw := []definition.Definition{early, late}

	resLIOS, conflictLIOS := detectKey(Key{Type: "common", ID: "thing"}, raw, dependencyGraph{}, LoadOrder{"mod_a"}, nil)
	if resLIOS.Reason != ReasonSingle {
		t.Errorf("LIOS: Reason = %v, want ReasonSingle (collapsed to one mod)", resLIOS.Reason)
	}
	if resLIOS.Winner.FilePath != late.FilePath {
		t.Errorf("LIOS: Winner.FilePath = %q, want the later file %q", resLIOS.Winner.FilePath, late.FilePath)
	}
	if conflictLIOS != nil {
		t.Errorf("a same-mod collapse must never produce a Conflict, got %+v", conflictLIOS)
	}

	rules := PriorityRules{"common": FIOS}
	resFIOS, _ := detectKey(Key{Type: "common", ID: "thing"}, raw, dependencyGraph{}, LoadOrder{"mod_a"}, rules)
	if resFIOS.Winner.FilePath != early.FilePath {
		t.Errorf("FIOS: Winner.FilePath = %q, want the earlier file %q", resFIOS.Winner.FilePath, early.FilePath)
	}
}
