package launch

import (
	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// DLCLoad is dlc_load.json's shape for classic-descriptor games: an
// ordered enabled-mods list and a disabled-DLC list. Confirmed
// byte-for-byte against a real Stellaris install (see
// docs/game-launching.md) - this is the file's entire contents, so a full
// overwrite is correct here.
type DLCLoad struct {
	EnabledMods  []string `json:"enabled_mods"`
	DisabledDLCs []string `json:"disabled_dlcs"`
}

// writeClassicState writes dlc_load.json, mods_registry.json, and
// game_data.json under opts.StateDir - dlc_load.json is what actually,
// primarily controls which mods the game loads, so it's written first and
// on its own once everything else has been prepared and validated: a
// failure partway through updateModsRegistry/updateGameData below (a
// corrupt existing file, say) aborts before anything touches disk, and a
// failure during the writes themselves still leaves dlc_load.json - the
// file that actually matters for the game to launch correctly - either
// fully written or fully untouched, never half of one and half the other.
//
// mods_registry.json and game_data.json are read-merge-write, not a full
// overwrite like dlc_load.json: mods_registry.json tracks every mod this
// project (or Steam/the launcher) has ever seen, not just this launch's
// enabled set, and game_data.json carries at least an isEulaAccepted flag
// alongside modsOrder - see updateModsRegistry/updateGameData for why
// blindly overwriting either would be a real regression, not just untidy.
func writeClassicState(enabledMods []mod.Mod, opts Options) (Result, error) {
	disabled := opts.DisabledDLC
	if disabled == nil {
		disabled = []string{}
	}
	entries := make([]string, len(enabledMods))
	for i, m := range enabledMods {
		entries[i] = classicDescriptorPath(m.ID)
	}
	dlcLoad := DLCLoad{EnabledMods: entries, DisabledDLCs: disabled}

	registry, uuidByModID, err := updateModsRegistry(opts.StateDir, enabledMods)
	if err != nil {
		return Result{}, err
	}
	modsOrder := make([]string, len(enabledMods))
	for i, m := range enabledMods {
		modsOrder[i] = uuidByModID[m.ID]
	}
	gameData, err := updateGameData(opts.StateDir, modsOrder)
	if err != nil {
		return Result{}, err
	}

	dlcLoadPath, err := atomicfile.WriteJSON(opts.StateDir, "dlc_load.json", dlcLoad)
	if err != nil {
		return Result{}, err
	}
	registryPath, err := atomicfile.WriteJSON(opts.StateDir, modsRegistryFilename, registry)
	if err != nil {
		return Result{}, err
	}
	gameDataPath, err := atomicfile.WriteJSON(opts.StateDir, gameDataFilename, gameData)
	if err != nil {
		return Result{}, err
	}

	return Result{Written: []string{dlcLoadPath, registryPath, gameDataPath}}, nil
}
