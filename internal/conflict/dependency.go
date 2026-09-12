package conflict

// dependencyGraph is an undirected adjacency map, modID -> otherModID ->
// the concrete declared DependencyEdge that connects them, per
// buildDependencyGraph. Storing the original directed edge under both
// lookup directions means a query from either side returns a real declared
// dependency, never a fabricated reverse one.
type dependencyGraph map[string]map[string]DependencyEdge

func (g dependencyGraph) addEdge(from, to string) {
	edge := DependencyEdge{FromModID: from, ToModID: to}
	if g[from] == nil {
		g[from] = map[string]DependencyEdge{}
	}
	if g[to] == nil {
		g[to] = map[string]DependencyEdge{}
	}
	g[from][to] = edge
	g[to][from] = edge
}

func (g dependencyGraph) connected(a, b string) bool {
	_, ok := g[a][b]
	return ok
}

func (g dependencyGraph) edge(a, b string) (DependencyEdge, bool) {
	e, ok := g[a][b]
	return e, ok
}

// fullyConnected reports whether every pair of distinct ids is connected in
// g - the bar for suppressing a conflict among them. For exactly 2 ids
// this reduces to a single edge check; for 3+, a conflict a dependency
// graph only partially explains still isn't suppressed, since silently
// hiding a conflict the manager can't fully account for would undercut the
// whole point of the app (see docs/conflict-resolution.md).
func (g dependencyGraph) fullyConnected(ids []string) bool {
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			if !g.connected(ids[i], ids[j]) {
				return false
			}
		}
	}
	return true
}

// buildDependencyGraph resolves every input's Descriptor.Dependencies
// (free-text names - see internal/mod/descriptor.go) against every other
// input's Descriptor.Name (falling back to Descriptor.ID for JSON-format
// mods, whose Name may legitimately be empty) into an undirected
// modID<->modID graph. A dependency name that doesn't match any input mod
// is dropped - the mod it would name isn't part of this Resolve run, so it
// can't explain anything. Matching is exact-string only; this is a
// documented limitation, not a claim of fuzzy matching.
func buildDependencyGraph(inputs []Input) dependencyGraph {
	nameToID := make(map[string]string, len(inputs))
	for _, inp := range inputs {
		name := inp.Mod.Descriptor.Name
		if name == "" {
			name = inp.Mod.Descriptor.ID
		}
		if name != "" {
			nameToID[name] = inp.Mod.ID
		}
	}

	g := dependencyGraph{}
	for _, inp := range inputs {
		for _, depName := range inp.Mod.Descriptor.Dependencies {
			depID, ok := nameToID[depName]
			if !ok || depID == inp.Mod.ID {
				continue
			}
			g.addEdge(inp.Mod.ID, depID)
		}
	}
	return g
}
