package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

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
	if len(result.Written) != 1 || result.Written[0] != wantPath {
		t.Errorf("Result.Written = %v, want [%s]", result.Written, wantPath)
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

func TestWriteStateNeverWritesGameData(t *testing.T) {
	// game_data.json's modsOrder needs a mod-UUID registry this project
	// doesn't track yet (see docs/game-launching.md) - writing plain mod-ID
	// strings into that UUID-keyed field would be wrong, so WriteState must
	// never touch this file at all, not even if a stale copy already
	// exists from a previous real launcher run.
	dir := t.TempDir()
	preexisting := []byte(`{"isEulaAccepted": true, "modsOrder": ["21153e40-4eea-4b4e-bae1-59ec0ccc8016"]}`)
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
	if string(after) != string(preexisting) {
		t.Errorf("game_data.json was modified: %q, want untouched %q", after, preexisting)
	}
}
