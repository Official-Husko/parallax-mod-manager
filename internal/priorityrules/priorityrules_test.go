package priorityrules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyMap(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	rules := s.Load("stellaris")
	if rules == nil {
		t.Fatal("Load returned nil, want a non-nil empty map")
	}
	if len(rules) != 0 {
		t.Errorf("rules = %+v, want empty", rules)
	}
	if got := (Store{}).Load("stellaris"); len(got) != 0 {
		t.Errorf("no Dir at all: got %+v, want empty", got)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	want := map[string]string{
		"common/scripted_variables": FIOS,
		"common/buildings":          LIOS,
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
	if err := s.Save("stellaris", map[string]string{"common/a": FIOS}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save("stellaris", map[string]string{"common/b": LIOS}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := s.Load("stellaris")
	if _, ok := got["common/a"]; ok {
		t.Error("common/a still present after a replacing save, want it gone")
	}
	if got["common/b"] != LIOS {
		t.Errorf("got = %+v, want only common/b", got)
	}
}

func TestRulesAreScopedPerGame(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", map[string]string{"common/a": FIOS}); err != nil {
		t.Fatalf("Save stellaris: %v", err)
	}
	if err := s.Save("hoi4", map[string]string{"common/b": LIOS}); err != nil {
		t.Fatalf("Save hoi4: %v", err)
	}
	stellaris := s.Load("stellaris")
	hoi4 := s.Load("hoi4")
	if _, ok := stellaris["common/b"]; ok {
		t.Error("hoi4's own rule leaked into stellaris'")
	}
	if _, ok := hoi4["common/a"]; ok {
		t.Error("stellaris' own rule leaked into hoi4's")
	}
}

func TestSaveWithNoDirIsAnError(t *testing.T) {
	if err := (Store{}).Save("stellaris", map[string]string{"a": FIOS}); err == nil {
		t.Error("expected an error saving with no Dir set")
	}
}

func TestACorruptFileIsIgnoredNotAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stellaris.jsonc"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := (Store{Dir: dir}).Load("stellaris")
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty for a corrupt file", got)
	}
}

func TestHandWrittenJSONCIsRead(t *testing.T) {
	dir := t.TempDir()
	body := `// my own priority rule overrides
{
  "rules": {
    // I've seen this cause duplicate buildings in-game with later mods loaded
    "common/scripted_variables": "FIOS",
  },
}`
	if err := os.WriteFile(filepath.Join(dir, "stellaris.jsonc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := (Store{Dir: dir}).Load("stellaris")
	if got["common/scripted_variables"] != FIOS {
		t.Errorf("got = %+v, want common/scripted_variables: FIOS", got)
	}
}
