package modpins

import (
	"os"
	"testing"
)

func TestLoadMissingFileReturnsEmptySlice(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	ids := s.Load("stellaris")
	if ids == nil {
		t.Fatal("Load returned nil, want a non-nil empty slice")
	}
	if len(ids) != 0 {
		t.Errorf("ids = %+v, want empty", ids)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	want := []string{"ugc_123", "some_local_mod"}
	if err := s.Save("stellaris", want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := s.Load("stellaris")
	if len(got) != len(want) {
		t.Fatalf("got = %+v, want %+v", got, want)
	}
	gotSet := make(map[string]bool, len(got))
	for _, id := range got {
		gotSet[id] = true
	}
	for _, id := range want {
		if !gotSet[id] {
			t.Errorf("got = %+v, missing %q", got, id)
		}
	}
}

func TestSaveSortsForDeterministicOutput(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", []string{"zebra", "alpha", "mid"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := s.Load("stellaris")
	want := []string{"alpha", "mid", "zebra"}
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
	if err := s.Save("stellaris", []string{"mod_a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := s.Load("hoi4"); len(got) != 0 {
		t.Errorf("hoi4's pinned set = %+v, want empty (games must not share state)", got)
	}
}

func TestSaveEmptyDirErrors(t *testing.T) {
	s := Store{}
	if err := s.Save("stellaris", []string{"mod_a"}); err == nil {
		t.Error("Save with empty Dir succeeded, want an error")
	}
}

func TestLoadEmptyDirReturnsEmptySlice(t *testing.T) {
	s := Store{}
	if ids := s.Load("stellaris"); len(ids) != 0 {
		t.Errorf("ids = %+v, want empty", ids)
	}
}

func TestCorruptFileReturnsEmptySlice(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", []string{"mod_a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.WriteFile(s.path("stellaris"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := s.Load("stellaris"); len(got) != 0 {
		t.Errorf("ids = %+v, want empty for a corrupt file", got)
	}
}
