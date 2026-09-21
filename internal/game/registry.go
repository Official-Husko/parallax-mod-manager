package game

import (
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// Stellaris is a stable, well-known fixture for this module's tests only -
// the running app never uses it. The real registry always comes from
// data/games.jsonc via LoadRegistry; this stays hand-kept in sync with
// that file's Stellaris entry (same ID) purely so every package's test
// suite has one convenient, realistic classic-descriptor GameConfig to
// build on without needing its own embed access to data/ (go:embed can't
// reach outside internal/game's own directory tree, which is exactly why
// this can't just be "parse the embedded default at init time" instead).
var Stellaris = GameConfig{
	ID:                   "01a0963a-c214-75a3-908d-1b76b91ea7bf",
	DisplayName:          "Stellaris",
	SteamAppID:           "281990",
	FolderName:           "Stellaris",
	DescriptorType:       mod.DescriptorClassic,
	LauncherSettingsPath: "launcher-settings.json",
	SignatureFiles:       []string{"launcher-settings.json"},
	ScanFolders: []string{
		"common", "events", "map", "localisation", "gfx", "gui",
		"prescripted_countries", "solar_system_initializers",
	},
	ChecksumAlgorithm: "stellaris",
	DLC:               []DLCEntry{}, // matches data/games.jsonc's "dlc": [] - see TestRealGamesListFileIsValidAndMatchesTheFixture
}

// Registry looks up GameConfig by ID. The games it holds come from
// LoadRegistry, not from literals in this file - see gamelist.go.
type Registry struct {
	games map[string]GameConfig
}

// NewRegistry returns a Registry holding exactly the given games.
func NewRegistry(games []GameConfig) *Registry {
	r := &Registry{games: map[string]GameConfig{}}
	for _, g := range games {
		r.register(g)
	}
	return r
}

func (r *Registry) register(g GameConfig) {
	r.games[g.ID] = g
}

// Get looks up a game by its UUID.
func (r *Registry) Get(id string) (GameConfig, bool) {
	g, ok := r.games[id]
	return g, ok
}

// List returns every registered game, sorted by DisplayName for
// deterministic output (map iteration order is not stable) - for a game
// picker.
func (r *Registry) List() []GameConfig {
	games := make([]GameConfig, 0, len(r.games))
	for _, g := range r.games {
		games = append(games, g)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].DisplayName < games[j].DisplayName })
	return games
}
