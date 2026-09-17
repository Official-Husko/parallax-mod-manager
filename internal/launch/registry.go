package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

const (
	modsRegistryFilename = "mods_registry.json"
	gameDataFilename     = "game_data.json"
)

// RegistryEntry is one mods_registry.json entry, keyed by its own real
// UUID - confirmed byte-for-byte against a real Stellaris install (see
// docs/game-launching.md). game_data.json's modsOrder references mods by
// this UUID, not by dlc_load.json's own descriptor-path identifier -
// resolving that bridge is this file's whole job.
type RegistryEntry struct {
	DirPath         string   `json:"dirPath"`
	DisplayName     string   `json:"displayName"`
	GameRegistryID  string   `json:"gameRegistryId"`
	ID              string   `json:"id"`
	RequiredVersion string   `json:"requiredVersion"`
	Source          string   `json:"source"`
	Status          string   `json:"status"`
	SteamID         string   `json:"steamId,omitempty"`
	Tags            []string `json:"tags"`
}

// loadRawRegistry reads dir's real mods_registry.json, if it exists, as
// raw per-UUID entries rather than unmarshaling into RegistryEntry - so
// any entry this launch never touches (another mod entirely, or a field
// this project has never had reason to model on one it does) round-trips
// completely untouched. A missing file isn't an error - a first launch,
// or a fresh UserDataDir - just an empty registry to build up from; a
// file that exists but won't parse as a JSON object is, since silently
// starting over would mean throwing away real, currently-working launcher
// state instead of surfacing the problem.
func loadRawRegistry(dir string) (map[string]json.RawMessage, error) {
	raw, err := os.ReadFile(filepath.Join(dir, modsRegistryFilename))
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("launch: reading existing %s: %w", modsRegistryFilename, err)
	}
	reg := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &reg); err != nil {
		return nil, fmt.Errorf("launch: parsing existing %s: %w", modsRegistryFilename, err)
	}
	return reg, nil
}

// existingUUIDs indexes reg by each entry's own gameRegistryId (the exact
// "mod/<id>.mod" string dlc_load.json also uses - see
// classicDescriptorPath), so a mod that's already registered keeps its
// real UUID across launches instead of minting a fresh one every time.
// An entry that doesn't parse as at least a gameRegistryId is simply
// skipped - this is only a lookup aid for reuse, not validation of the
// file's own contents (loadRawRegistry already handles a genuinely
// unparseable file).
func existingUUIDs(reg map[string]json.RawMessage) map[string]string {
	out := make(map[string]string, len(reg))
	for id, raw := range reg {
		var partial struct {
			GameRegistryID string `json:"gameRegistryId"`
		}
		if json.Unmarshal(raw, &partial) == nil && partial.GameRegistryID != "" {
			out[partial.GameRegistryID] = id
		}
	}
	return out
}

// sourceString maps mod.Source to mods_registry.json's own "source"
// values, matching the same Workshop/Paradox-launcher/local split
// docs/mod-sources.md already documents for classification.
func sourceString(s mod.Source) string {
	switch s {
	case mod.SourceWorkshop:
		return "steam"
	case mod.SourceParadoxLauncher:
		return "pdx"
	default:
		return "local"
	}
}

// requiredVersion is the game-version-compatibility string
// mods_registry.json expects for one mod - its own declared
// supported_version when it set one, falling back to its plain version
// field, and finally "*" (any version) when it set neither, matching a
// real confirmed entry for a mod that declared neither.
func requiredVersion(d mod.Descriptor) string {
	if d.SupportedVersion != "" {
		return d.SupportedVersion
	}
	if d.Version != "" {
		return d.Version
	}
	return "*"
}

// updateModsRegistry merges enabledMods into dir's real mods_registry.json
// (reading and preserving whatever's already there - see loadRawRegistry),
// reusing each mod's existing UUID when one's already registered under its
// gameRegistryId and minting a fresh one otherwise. Returns the full
// updated registry, ready to write, and a modID -> UUID map for building
// game_data.json's modsOrder in the same load order.
func updateModsRegistry(dir string, enabledMods []mod.Mod) (map[string]json.RawMessage, map[string]string, error) {
	reg, err := loadRawRegistry(dir)
	if err != nil {
		return nil, nil, err
	}
	known := existingUUIDs(reg)

	uuidByModID := make(map[string]string, len(enabledMods))
	for _, m := range enabledMods {
		gameRegistryID := classicDescriptorPath(m.ID)

		id, reused := known[gameRegistryID]
		if !reused {
			id = uuid.NewString()
		}
		uuidByModID[m.ID] = id

		tags := m.Descriptor.Tags
		if tags == nil {
			tags = []string{}
		}
		entry := RegistryEntry{
			DirPath:         m.ContentPath,
			DisplayName:     m.Descriptor.Name,
			GameRegistryID:  gameRegistryID,
			ID:              id,
			RequiredVersion: requiredVersion(m.Descriptor),
			Source:          sourceString(m.Source),
			Status:          "ready_to_play",
			Tags:            tags,
		}
		if m.Source == mod.SourceWorkshop {
			entry.SteamID = m.Descriptor.RemoteFileID
		}

		encoded, err := json.Marshal(entry)
		if err != nil {
			return nil, nil, fmt.Errorf("launch: encoding registry entry for %s: %w", m.ID, err)
		}
		reg[id] = encoded
	}

	return reg, uuidByModID, nil
}

// updateGameData reads dir's real game_data.json, if present, preserving
// every field it doesn't know about - a real confirmed install carries at
// least isEulaAccepted alongside modsOrder (see docs/game-launching.md),
// and blindly overwriting the whole file would silently reset that flag,
// likely reshowing the EULA prompt the next time the game starts - and
// returns it with only modsOrder replaced. A missing file isn't an error,
// same reasoning as loadRawRegistry above; a file that exists but won't
// parse as a JSON object is.
func updateGameData(dir string, modsOrder []string) (map[string]any, error) {
	data := map[string]any{}
	raw, err := os.ReadFile(filepath.Join(dir, gameDataFilename))
	switch {
	case err == nil:
		if jsonErr := json.Unmarshal(raw, &data); jsonErr != nil {
			return nil, fmt.Errorf("launch: parsing existing %s: %w", gameDataFilename, jsonErr)
		}
	case !os.IsNotExist(err):
		return nil, fmt.Errorf("launch: reading existing %s: %w", gameDataFilename, err)
	}
	data["modsOrder"] = modsOrder
	return data, nil
}
