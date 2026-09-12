package game

import (
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// Stellaris is the seed GameConfig for the first supported game. Other games
// get added here (or loaded from data) as they're supported - see
// docs/game-configuration.md for what each field means and why.
var Stellaris = GameConfig{
	Key:                  "stellaris",
	DisplayName:          "Stellaris",
	SteamAppID:           "281990",
	FolderName:           "Stellaris",
	DescriptorType:       mod.DescriptorClassic,
	LauncherSettingsPath: "launcher-settings.json",
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

// CrusaderKings3, EuropaUniversalis4, HeartsOfIron4, ImperatorRome, and
// Victoria3 are seeded from cross-checking Steam app ids and per-game
// content-folder layouts against publicly documented Paradox game data
// (not verified against a real install of each on this machine the way
// Stellaris has been - see each game's own comment below for what's still
// an assumption).
//
// launcher-settings.json's location isn't uniform: most games keep it at
// the install root, but a few nest it under a "launcher/" subfolder - that
// difference is what LauncherSettingsPath exists to capture per game,
// rather than assuming Stellaris's layout everywhere.

var CrusaderKings3 = GameConfig{
	Key:                  "ck3",
	DisplayName:          "Crusader Kings III",
	SteamAppID:           "1158310",
	FolderName:           "Crusader Kings III",
	DescriptorType:       mod.DescriptorClassic,
	LauncherSettingsPath: "launcher/launcher-settings.json",
	SignatureFiles:       []string{"launcher/launcher-settings.json"},
	ScanFolders:          []string{"common", "events", "history", "map_data", "gui", "localization", "data_binding"},
}

var EuropaUniversalis4 = GameConfig{
	Key:                  "eu4",
	DisplayName:          "Europa Universalis IV",
	SteamAppID:           "236850",
	FolderName:           "Europa Universalis IV",
	DescriptorType:       mod.DescriptorClassic,
	LauncherSettingsPath: "launcher-settings.json",
	SignatureFiles:       []string{"launcher-settings.json"},
	ScanFolders:          []string{"common", "events", "missions", "decisions", "history", "map"},
}

var HeartsOfIron4 = GameConfig{
	Key:                  "hoi4",
	DisplayName:          "Hearts of Iron IV",
	SteamAppID:           "394360",
	FolderName:           "Hearts of Iron IV",
	DescriptorType:       mod.DescriptorClassic,
	LauncherSettingsPath: "launcher-settings.json",
	SignatureFiles:       []string{"launcher-settings.json"},
	ScanFolders:          []string{"common", "events", "history", "map"},
}

var ImperatorRome = GameConfig{
	Key:                  "imperator",
	DisplayName:          "Imperator: Rome",
	SteamAppID:           "859580",
	FolderName:           "Imperator",
	DescriptorType:       mod.DescriptorClassic,
	LauncherSettingsPath: "launcher/launcher-settings.json",
	SignatureFiles:       []string{"launcher/launcher-settings.json"},
	ScanFolders:          []string{"common", "events", "decisions", "gui", "localization", "map_data", "setup"},
}

// Victoria3 uses the newer Paradox Launcher JSON descriptor format rather
// than classic descriptor.mod - DescriptorJSONv1 is a best guess between
// the two relationship-shape variants that format supports; this project
// hasn't yet confirmed which one against a real Victoria 3 install (see
// README's "Not yet built" list).
var Victoria3 = GameConfig{
	Key:                  "vic3",
	DisplayName:          "Victoria 3",
	SteamAppID:           "529340",
	FolderName:           "Victoria 3",
	DescriptorType:       mod.DescriptorJSONv1,
	LauncherSettingsPath: "launcher/launcher-settings.json",
	SignatureFiles:       []string{"launcher/launcher-settings.json"},
	ScanFolders:          []string{"common", "events", "map_data", "gui", "localization"},
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
	r.register(CrusaderKings3)
	r.register(EuropaUniversalis4)
	r.register(HeartsOfIron4)
	r.register(ImperatorRome)
	r.register(Victoria3)
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
