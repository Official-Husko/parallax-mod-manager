package library

import (
	"reflect"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func TestSourceString(t *testing.T) {
	tests := []struct {
		source mod.Source
		want   string
	}{
		{mod.SourceLocal, "local"},
		{mod.SourceWorkshop, "workshop"},
		{mod.SourceParadoxLauncher, "paradox-launcher"},
	}
	for _, tt := range tests {
		if got := sourceString(tt.source); got != tt.want {
			t.Errorf("sourceString(%v) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestDisplayNameFallsBackToID(t *testing.T) {
	named := mod.Mod{ID: "some_id", Descriptor: mod.Descriptor{Name: "Some Mod"}}
	if got := displayName(named); got != "Some Mod" {
		t.Errorf("displayName(named) = %q, want %q", got, "Some Mod")
	}

	unnamed := mod.Mod{ID: "some_id"}
	if got := displayName(unnamed); got != "some_id" {
		t.Errorf("displayName(unnamed) = %q, want %q (fallback to ID)", got, "some_id")
	}
}

func TestBuildConflictSummariesResolvesNames(t *testing.T) {
	conflicts := []conflict.Conflict{
		{
			Key: conflict.Key{Type: "common/buildings", ID: "some_building"},
			Candidates: []definition.Definition{
				{ModID: "mod_a"},
				{ModID: "mod_b"},
			},
		},
	}
	names := map[string]string{"mod_a": "Mod A", "mod_b": "Mod B"}

	got := buildConflictSummaries(conflicts, names)
	want := []ConflictSummary{
		{Type: "common/buildings", ID: "some_building", Candidates: []string{"Mod A", "Mod B"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildConflictSummaries = %+v, want %+v", got, want)
	}
}

func TestBuildConflictSummariesFallsBackToIDWhenNameMissing(t *testing.T) {
	conflicts := []conflict.Conflict{
		{
			Key:        conflict.Key{Type: "common", ID: "thing"},
			Candidates: []definition.Definition{{ModID: "unknown_mod"}},
		},
	}
	got := buildConflictSummaries(conflicts, map[string]string{})
	if len(got) != 1 || len(got[0].Candidates) != 1 || got[0].Candidates[0] != "unknown_mod" {
		t.Errorf("buildConflictSummaries = %+v, want candidate to fall back to ModID", got)
	}
}

func TestBuildConflictSummariesEmptyInput(t *testing.T) {
	got := buildConflictSummaries(nil, nil)
	if len(got) != 0 {
		t.Errorf("buildConflictSummaries(nil, nil) = %+v, want empty", got)
	}
}
