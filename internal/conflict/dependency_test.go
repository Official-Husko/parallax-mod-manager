package conflict

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func inputWith(id, name string, deps ...string) Input {
	return Input{Mod: mod.Mod{ID: id, Descriptor: mod.Descriptor{Name: name, Dependencies: deps}}}
}

func TestBuildDependencyGraphResolvesViaName(t *testing.T) {
	inputs := []Input{
		inputWith("mod_base", "Base Mod"),
		inputWith("mod_patch", "Patch For Base", "Base Mod"),
	}
	g := buildDependencyGraph(inputs)
	if !g.connected("mod_base", "mod_patch") {
		t.Error("expected mod_base and mod_patch to be connected via the declared dependency name")
	}
	if !g.connected("mod_patch", "mod_base") {
		t.Error("expected the connection to be queryable from either direction")
	}
}

func TestBuildDependencyGraphResolvesViaJSONIDFallback(t *testing.T) {
	// A JSON-format mod with no Name set, only an ID.
	depender := Input{Mod: mod.Mod{ID: "mod_patch", Descriptor: mod.Descriptor{Dependencies: []string{"base-id"}}}}
	base := Input{Mod: mod.Mod{ID: "mod_base", Descriptor: mod.Descriptor{ID: "base-id"}}}

	g := buildDependencyGraph([]Input{depender, base})
	if !g.connected("mod_patch", "mod_base") {
		t.Error("expected the dependency name to resolve via the JSON ID fallback")
	}
}

func TestBuildDependencyGraphDropsUnresolvableNames(t *testing.T) {
	inputs := []Input{
		inputWith("mod_a", "Mod A", "Some Mod That Isn't Enabled"),
	}
	g := buildDependencyGraph(inputs)
	if len(g) != 0 {
		t.Errorf("expected an unresolvable dependency name to be dropped without creating any edge, got %+v", g)
	}
}

func TestBuildDependencyGraphIgnoresSelfDependency(t *testing.T) {
	inputs := []Input{inputWith("mod_a", "Mod A", "Mod A")}
	g := buildDependencyGraph(inputs)
	if g.connected("mod_a", "mod_a") {
		t.Error("a mod should never be recorded as depending on itself")
	}
}

func TestDependencyGraphEdgeReturnsDeclaredDirection(t *testing.T) {
	inputs := []Input{
		inputWith("mod_base", "Base Mod"),
		inputWith("mod_patch", "Patch", "Base Mod"),
	}
	g := buildDependencyGraph(inputs)

	edge, ok := g.edge("mod_patch", "mod_base")
	if !ok {
		t.Fatal("expected an edge to be found")
	}
	if edge.FromModID != "mod_patch" || edge.ToModID != "mod_base" {
		t.Errorf("edge = %+v, want FromModID=mod_patch ToModID=mod_base (mod_patch declared the dependency)", edge)
	}

	// Querying from the other side should return the SAME declared edge,
	// not a fabricated reverse one.
	reverseQueryEdge, ok := g.edge("mod_base", "mod_patch")
	if !ok || reverseQueryEdge != edge {
		t.Errorf("edge queried from the other side = %+v, want the identical declared edge %+v", reverseQueryEdge, edge)
	}
}

func TestFullyConnectedTwoMods(t *testing.T) {
	inputs := []Input{
		inputWith("mod_a", "Mod A"),
		inputWith("mod_b", "Mod B", "Mod A"),
	}
	g := buildDependencyGraph(inputs)

	if !g.fullyConnected([]string{"mod_a", "mod_b"}) {
		t.Error("expected two directly-connected mods to be fully connected")
	}
	if g.fullyConnected([]string{"mod_a", "mod_c"}) {
		t.Error("expected an unconnected pair to not be fully connected")
	}
}

func TestFullyConnectedThreeModsRequiresEveryPair(t *testing.T) {
	// mod_a <-> mod_b, and mod_b <-> mod_c, but mod_a and mod_c have no
	// direct relationship - this must NOT count as fully connected: a
	// conflict a dependency graph only partially explains still needs to
	// surface to the user (see docs/conflict-resolution.md and this
	// package's doc comments on the suppression bar).
	inputs := []Input{
		inputWith("mod_a", "Mod A"),
		inputWith("mod_b", "Mod B", "Mod A"),
		inputWith("mod_c", "Mod C", "Mod B"),
	}
	g := buildDependencyGraph(inputs)

	if g.fullyConnected([]string{"mod_a", "mod_b", "mod_c"}) {
		t.Error("a chain (a-b, b-c, no a-c) must not count as fully connected")
	}

	// Now add the missing edge - should become fully connected.
	inputsFull := []Input{
		inputWith("mod_a", "Mod A", "Mod C"),
		inputWith("mod_b", "Mod B", "Mod A"),
		inputWith("mod_c", "Mod C", "Mod B"),
	}
	gFull := buildDependencyGraph(inputsFull)
	if !gFull.fullyConnected([]string{"mod_a", "mod_b", "mod_c"}) {
		t.Error("expected a fully-connected triangle to be reported as fully connected")
	}
}
