package launch

import "github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"

// DLCLoad is dlc_load.json's shape for classic-descriptor games: an
// ordered enabled-mods list and a disabled-DLC list. Confirmed
// byte-for-byte against a real Stellaris install (see
// docs/game-launching.md) - this is the file's entire contents, so a full
// overwrite is correct here.
type DLCLoad struct {
	EnabledMods  []string `json:"enabled_mods"`
	DisabledDLCs []string `json:"disabled_dlcs"`
}

// writeClassicState writes dlc_load.json under opts.StateDir.
//
// game_data.json and mods_registry.json are deliberately not written.
// Confirmed against a real Stellaris install (see docs/game-launching.md):
// game_data.json's modsOrder holds mod UUIDs from mods_registry.json, not
// the descriptor-path strings dlc_load.json uses - a different identifier
// space entirely. Writing it correctly means resolving each mod to a
// registry UUID first (reusing an existing mods_registry.json entry's UUID
// when one exists, minting a fresh one otherwise), which this project
// doesn't track yet. Writing plain mod-ID strings into a UUID-keyed field
// would be objectively wrong, not just unverified, so this stays
// unimplemented until UUID tracking exists rather than shipping a write
// path known to corrupt that field.
func writeClassicState(enabledMods []string, opts Options) (Result, error) {
	disabled := opts.DisabledDLC
	if disabled == nil {
		disabled = []string{}
	}
	dlcLoad := DLCLoad{EnabledMods: enabledMods, DisabledDLCs: disabled}

	dlcLoadPath, err := atomicfile.WriteJSON(opts.StateDir, "dlc_load.json", dlcLoad)
	if err != nil {
		return Result{}, err
	}
	return Result{Written: []string{dlcLoadPath}}, nil
}
