package patchoverride

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyMap(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	overrides := s.Load("stellaris")
	if overrides == nil {
		t.Fatal("Load returned nil, want a non-nil empty map")
	}
	if len(overrides) != 0 {
		t.Errorf("overrides = %+v, want empty", overrides)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	want := map[string]string{
		Key("common/buildings", "building_capital"): "mod_a",
		Key("common/species_rights", "default"):     "mod_b",
	}
	if err := s.Save("stellaris", want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := s.Load("stellaris")
	if len(got) != len(want) {
		t.Fatalf("got = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestSaveReplacesWholeSet(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", map[string]string{"a:1": "mod_a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save("stellaris", map[string]string{"b:2": "mod_b"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := s.Load("stellaris")
	if _, ok := got["a:1"]; ok {
		t.Error("a:1 still present after a replacing save, want it gone")
	}
	if got["b:2"] != "mod_b" {
		t.Errorf("got = %+v, want only b:2", got)
	}
}

func TestOverridesAreScopedPerGame(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", map[string]string{"a:1": "mod_a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save("eu4", map[string]string{"a:1": "mod_b"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := s.Load("stellaris"); got["a:1"] != "mod_a" {
		t.Errorf("stellaris override = %+v", got)
	}
	if got := s.Load("eu4"); got["a:1"] != "mod_b" {
		t.Errorf("eu4 override = %+v", got)
	}
}

func TestLoadCorruptFileReturnsEmptyMapNotError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stellaris.jsonc"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("writing corrupt file: %v", err)
	}
	s := Store{Dir: dir}
	overrides := s.Load("stellaris")
	if len(overrides) != 0 {
		t.Errorf("overrides = %+v, want empty for a corrupt file", overrides)
	}
}

func TestLoadToleratesHandWrittenComments(t *testing.T) {
	dir := t.TempDir()
	content := `{
		// I picked mod_a here because it fixes the crash from mod_b's version
		"overrides": {
			"common/buildings:building_capital": "mod_a", // trailing comma below is fine too
		},
	}`
	if err := os.WriteFile(filepath.Join(dir, "stellaris.jsonc"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	s := Store{Dir: dir}
	overrides := s.Load("stellaris")
	if overrides["common/buildings:building_capital"] != "mod_a" {
		t.Errorf("overrides = %+v", overrides)
	}
}

func TestSaveWithoutDirErrors(t *testing.T) {
	s := Store{}
	if err := s.Save("stellaris", map[string]string{"a:1": "mod_a"}); err == nil {
		t.Fatal("expected an error when Dir is unset")
	}
}
