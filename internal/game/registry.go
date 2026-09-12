package game

import (
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// Stellaris is the seed GameConfig for the first supported game. Other games
// get added here (or loaded from data) as they're supported - see
// docs/game-configuration.md for what each field means and why.
var Stellaris = GameConfig{
	Key:            "stellaris",
	DisplayName:    "Stellaris",
	SteamAppID:     "281990",
	FolderName:     "Stellaris",
	DescriptorType: mod.DescriptorClassic,
	// launcher-settings.json, not the game binary itself: confirmed against
	// a real Linux Stellaris install that the executable there is the
	// extension-less native binary "stellaris", not "stellaris.exe" - a
	// signature file needs to exist under the same name on every platform.
	SignatureFiles: []string{"launcher-settings.json"},
	ScanFolders: []string{
		"common", "events", "map", "localisation", "gfx", "gui",
		"prescripted_countries", "solar_system_initializers",
	},
}

// Registry looks up GameConfig by key.
type Registry struct {
	games map[string]GameConfig
}

// NewRegistry returns a Registry seeded with every game this project
// currently supports.
func NewRegistry() *Registry {
	r := &Registry{games: map[string]GameConfig{}}
	r.register(Stellaris)
	return r
}

func (r *Registry) register(g GameConfig) {
	r.games[g.Key] = g
}

// Get looks up a game by its short key (e.g. "stellaris").
func (r *Registry) Get(key string) (GameConfig, bool) {
	g, ok := r.games[key]
	return g, ok
}

// List returns every registered game, sorted by Key for deterministic
// output (map iteration order is not stable) - for a game picker.
func (r *Registry) List() []GameConfig {
	games := make([]GameConfig, 0, len(r.games))
	for _, g := range r.games {
		games = append(games, g)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].Key < games[j].Key })
	return games
}
