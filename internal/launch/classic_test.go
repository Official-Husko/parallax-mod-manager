package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// Atomic-write mechanics (round-trip, no leftover temp file, overwrite) are
// tested once, centrally, in internal/atomicfile - not duplicated here.

// --- WriteState (dlc_load.json) ---------------------------------------

func TestWriteStateWritesExpectedDLCLoad(t *testing.T) {
	dir := t.TempDir()
	mods := []mod.Mod{{ID: "mod_a"}, {ID: "mod_b"}}
	order := conflict.LoadOrder{"mod_a", "mod_b"}

	result, err := WriteState(order, mods, testClassicGame(), Options{
		StateDir:    dir,
		DisabledDLC: []string{"some_dlc"},
	})
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	wantPath := filepath.Join(dir, "dlc_load.json")
	wantWritten := []string{
		wantPath,
		filepath.Join(dir, "mods_registry.json"),
		filepath.Join(dir, "game_data.json"),
	}
	if !reflect.DeepEqual(result.Written, wantWritten) {
		t.Errorf("Result.Written = %v, want %v", result.Written, wantWritten)
	}

	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got DLCLoad
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := DLCLoad{
		EnabledMods:  []string{"mod/mod_a.mod", "mod/mod_b.mod"},
		DisabledDLCs: []string{"some_dlc"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DLCLoad = %+v, want %+v", got, want)
	}
}

func TestWriteStateEmptyListsMarshalAsEmptyArraysNotNull(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteState(conflict.LoadOrder{}, nil, testClassicGame(), Options{StateDir: dir}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "dlc_load.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Raw string check: json.Marshal(nil slice) would produce "null", not
	// "[]" - this is exactly the bug being guarded against.
	raw := string(data)
	if !jsonContainsEmptyArray(t, raw, "enabled_mods") || !jsonContainsEmptyArray(t, raw, "disabled_dlcs") {
		t.Errorf("expected empty-array (not null) fields in: %s", raw)
	}
}

func jsonContainsEmptyArray(t *testing.T, raw, field string) bool {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return string(m[field]) == "[]"
}

func TestWriteStateUnknownModPropagatesError(t *testing.T) {
	dir := t.TempDir()
	order := conflict.LoadOrder{"ghost_mod"}

	_, err := WriteState(order, nil, testClassicGame(), Options{StateDir: dir})
	if err == nil {
		t.Fatal("expected an error for an unknown mod in the load order")
	}
	// Nothing should have been written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected no files written on validation failure, got %+v", entries)
	}
}

func TestWriteStateUpdatesGameDataPreservingUnknownFields(t *testing.T) {
	// game_data.json carries at least isEulaAccepted alongside modsOrder
	// (see docs/game-launching.md) - WriteState must update modsOrder with
	// real registry UUIDs without resetting isEulaAccepted (or dropping any
	// other field a real launcher run might have written) back to its zero
	// value.
	dir := t.TempDir()
	preexisting := []byte(`{"isEulaAccepted": true, "someOtherField": "keep me", "modsOrder": ["stale-uuid"]}`)
	if err := os.WriteFile(filepath.Join(dir, "game_data.json"), preexisting, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mods := []mod.Mod{{ID: "mod_a"}}
	order := conflict.LoadOrder{"mod_a"}
	if _, err := WriteState(order, mods, testClassicGame(), Options{StateDir: dir}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(dir, "game_data.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(after, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["isEulaAccepted"] != true {
		t.Errorf("isEulaAccepted = %v, want true (preserved)", got["isEulaAccepted"])
	}
	if got["someOtherField"] != "keep me" {
		t.Errorf("someOtherField = %v, want %q (preserved)", got["someOtherField"], "keep me")
	}
	modsOrder, ok := got["modsOrder"].([]any)
	if !ok || len(modsOrder) != 1 {
		t.Fatalf("modsOrder = %v, want exactly one entry", got["modsOrder"])
	}
	newUUID, ok := modsOrder[0].(string)
	if !ok || newUUID == "" || newUUID == "stale-uuid" {
		t.Errorf("modsOrder[0] = %v, want a real, freshly-resolved UUID", modsOrder[0])
	}
	if _, err := uuid.Parse(newUUID); err != nil {
		t.Errorf("modsOrder[0] = %q, not a valid UUID: %v", newUUID, err)
	}
}

func TestWriteStateReusesSameUUIDAcrossLaunches(t *testing.T) {
	// A mod's registry UUID must stay stable launch to launch - Steam or
	// the game itself may reference it elsewhere, and minting a fresh one
	// every time would make mods_registry.json (and anything keyed off it)
	// churn for no reason.
	dir := t.TempDir()
	mods := []mod.Mod{{ID: "ugc_123", Source: mod.SourceWorkshop, Descriptor: mod.Descriptor{Name: "Test", RemoteFileID: "123"}}}
	order := conflict.LoadOrder{"ugc_123"}

	if _, err := WriteState(order, mods, testClassicGame(), Options{StateDir: dir}); err != nil {
		t.Fatalf("first WriteState: %v", err)
	}
	firstUUID := readModsOrder(t, dir)[0]

	if _, err := WriteState(order, mods, testClassicGame(), Options{StateDir: dir}); err != nil {
		t.Fatalf("second WriteState: %v", err)
	}
	secondUUID := readModsOrder(t, dir)[0]

	if firstUUID != secondUUID {
		t.Errorf("uuid changed across launches: %q then %q, want the same both times", firstUUID, secondUUID)
	}
}

func readModsOrder(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "game_data.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got struct {
		ModsOrder []string `json:"modsOrder"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return got.ModsOrder
}
