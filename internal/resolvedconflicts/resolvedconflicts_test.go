package resolvedconflicts

import (
	"testing"
)

func TestKeyMatchesTypeColonID(t *testing.T) {
	got := Key("common/buildings", "some_building")
	want := "common/buildings:some_building"
	if got != want {
		t.Errorf("Key = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsEmptySlice(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	keys := s.Load("stellaris")
	if keys == nil {
		t.Fatal("Load returned nil, want a non-nil empty slice")
	}
	if len(keys) != 0 {
		t.Errorf("keys = %+v, want empty", keys)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	want := []string{Key("common", "a"), Key("events", "b")}
	if err := s.Save("stellaris", want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := s.Load("stellaris")
	if len(got) != len(want) {
		t.Fatalf("got = %+v, want %+v", got, want)
	}
	gotSet := make(map[string]bool, len(got))
	for _, k := range got {
		gotSet[k] = true
	}
	for _, k := range want {
		if !gotSet[k] {
			t.Errorf("got = %+v, missing %q", got, k)
		}
	}
}

func TestSaveSortsForDeterministicOutput(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", []string{"zebra:1", "alpha:1", "mid:1"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := s.Load("stellaris")
	want := []string{"alpha:1", "mid:1", "zebra:1"}
	if len(got) != len(want) {
		t.Fatalf("got = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got = %+v, want %+v", got, want)
			break
		}
	}
}

func TestLoadIsolatedPerGame(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", []string{"common:a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := s.Load("hoi4"); len(got) != 0 {
		t.Errorf("hoi4's resolved set = %+v, want empty (games must not share state)", got)
	}
}

func TestSaveEmptyDirErrors(t *testing.T) {
	s := Store{}
	if err := s.Save("stellaris", []string{"common:a"}); err == nil {
		t.Error("Save with empty Dir succeeded, want an error")
	}
}

func TestLoadEmptyDirReturnsEmptySlice(t *testing.T) {
	s := Store{}
	if keys := s.Load("stellaris"); len(keys) != 0 {
		t.Errorf("keys = %+v, want empty", keys)
	}
}
