package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// --- sourceString / requiredVersion ------------------------------------

func TestSourceStringMapping(t *testing.T) {
	cases := []struct {
		source mod.Source
		want   string
	}{
		{mod.SourceWorkshop, "steam"},
		{mod.SourceParadoxLauncher, "pdx"},
		{mod.SourceLocal, "local"},
	}
	for _, c := range cases {
		if got := sourceString(c.source); got != c.want {
			t.Errorf("sourceString(%v) = %q, want %q", c.source, got, c.want)
		}
	}
}

func TestRequiredVersionPrefersSupportedVersion(t *testing.T) {
	got := requiredVersion(mod.Descriptor{SupportedVersion: "3.9.*", Version: "1.2.0"})
	if got != "3.9.*" {
		t.Errorf("requiredVersion = %q, want %q", got, "3.9.*")
	}
}

func TestRequiredVersionFallsBackToVersion(t *testing.T) {
	got := requiredVersion(mod.Descriptor{Version: "1.2.0"})
	if got != "1.2.0" {
		t.Errorf("requiredVersion = %q, want %q", got, "1.2.0")
	}
}

func TestRequiredVersionFallsBackToWildcard(t *testing.T) {
	got := requiredVersion(mod.Descriptor{})
	if got != "*" {
		t.Errorf("requiredVersion = %q, want %q", got, "*")
	}
}

// --- updateModsRegistry --------------------------------------------------

func TestUpdateModsRegistryMintsFreshUUIDForNewMod(t *testing.T) {
	dir := t.TempDir()
	mods := []mod.Mod{{
		ID:          "ugc_123",
		Source:      mod.SourceWorkshop,
		ContentPath: "/steamapps/workshop/content/281990/123",
		Descriptor:  mod.Descriptor{Name: "Test Mod", RemoteFileID: "123", Tags: []string{"Graphics"}},
	}}

	reg, uuidByModID, err := updateModsRegistry(dir, mods)
	if err != nil {
		t.Fatalf("updateModsRegistry: %v", err)
	}
	id := uuidByModID["ugc_123"]
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("minted id %q is not a valid UUID: %v", id, err)
	}

	var entry RegistryEntry
	if err := json.Unmarshal(reg[id], &entry); err != nil {
		t.Fatalf("Unmarshal entry: %v", err)
	}
	want := RegistryEntry{
		DirPath:         "/steamapps/workshop/content/281990/123",
		DisplayName:     "Test Mod",
		GameRegistryID:  "mod/ugc_123.mod",
		ID:              id,
		RequiredVersion: "*",
		Source:          "steam",
		Status:          "ready_to_play",
		SteamID:         "123",
		Tags:            []string{"Graphics"},
	}
	if !reflect.DeepEqual(entry, want) {
		t.Errorf("entry = %+v, want %+v", entry, want)
	}
}

func TestUpdateModsRegistryReusesExistingUUIDAndUpdatesFields(t *testing.T) {
	dir := t.TempDir()
	const existingID = "21153e40-4eea-4b4e-bae1-59ec0ccc8016"
	preexisting := map[string]json.RawMessage{
		existingID: json.RawMessage(`{
			"dirPath": "/old/path",
			"displayName": "Old Name",
			"gameRegistryId": "mod/ugc_123.mod",
			"id": "21153e40-4eea-4b4e-bae1-59ec0ccc8016",
			"requiredVersion": "*",
			"source": "steam",
			"status": "ready_to_play",
			"steamId": "123",
			"tags": []
		}`),
	}
	writeRegistryFixture(t, dir, preexisting)

	mods := []mod.Mod{{
		ID:          "ugc_123",
		Source:      mod.SourceWorkshop,
		ContentPath: "/new/path",
		Descriptor:  mod.Descriptor{Name: "New Name", RemoteFileID: "123"},
	}}

	reg, uuidByModID, err := updateModsRegistry(dir, mods)
	if err != nil {
		t.Fatalf("updateModsRegistry: %v", err)
	}
	if uuidByModID["ugc_123"] != existingID {
		t.Errorf("uuid = %q, want the existing %q reused", uuidByModID["ugc_123"], existingID)
	}
	if len(reg) != 1 {
		t.Fatalf("reg has %d entries, want exactly 1 (same UUID updated in place)", len(reg))
	}
	var entry RegistryEntry
	if err := json.Unmarshal(reg[existingID], &entry); err != nil {
		t.Fatalf("Unmarshal entry: %v", err)
	}
	if entry.DisplayName != "New Name" || entry.DirPath != "/new/path" {
		t.Errorf("entry = %+v, want refreshed DisplayName/DirPath", entry)
	}
}

func TestUpdateModsRegistryPreservesUnrelatedEntries(t *testing.T) {
	dir := t.TempDir()
	const untouchedID = "untouched-uuid"
	untouchedRaw := json.RawMessage(`{"gameRegistryId":"mod/ugc_999.mod","displayName":"Someone Else's Mod","extraField":"kept verbatim"}`)
	writeRegistryFixture(t, dir, map[string]json.RawMessage{untouchedID: untouchedRaw})

	mods := []mod.Mod{{ID: "ugc_123", Descriptor: mod.Descriptor{Name: "Test"}}}
	reg, _, err := updateModsRegistry(dir, mods)
	if err != nil {
		t.Fatalf("updateModsRegistry: %v", err)
	}
	if string(reg[untouchedID]) != string(untouchedRaw) {
		t.Errorf("untouched entry = %s, want byte-for-byte unchanged %s", reg[untouchedID], untouchedRaw)
	}
	if len(reg) != 2 {
		t.Errorf("reg has %d entries, want 2 (untouched + the newly resolved one)", len(reg))
	}
}

func TestUpdateModsRegistryCorruptExistingFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, modsRegistryFilename), []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := updateModsRegistry(dir, []mod.Mod{{ID: "mod_a"}}); err == nil {
		t.Fatal("expected an error for a corrupt existing mods_registry.json, got nil")
	}
}

// --- updateGameData ------------------------------------------------------

func TestUpdateGameDataMissingFileStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	data, err := updateGameData(dir, []string{"a", "b"})
	if err != nil {
		t.Fatalf("updateGameData: %v", err)
	}
	if len(data) != 1 {
		t.Errorf("data = %+v, want exactly the modsOrder key", data)
	}
	order, ok := data["modsOrder"].([]string)
	if !ok || len(order) != 2 {
		t.Errorf("modsOrder = %v, want [a b]", data["modsOrder"])
	}
}

func TestUpdateGameDataCorruptExistingFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, gameDataFilename), []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := updateGameData(dir, nil); err == nil {
		t.Fatal("expected an error for a corrupt existing game_data.json, got nil")
	}
}

func writeRegistryFixture(t *testing.T, dir string, reg map[string]json.RawMessage) {
	t.Helper()
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatalf("Marshal fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, modsRegistryFilename), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
