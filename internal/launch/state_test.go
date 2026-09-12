package launch

import (
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func TestResolveEnabledModsEmptyOrder(t *testing.T) {
	entries, err := resolveEnabledMods(conflict.LoadOrder{}, nil)
	if err != nil {
		t.Fatalf("resolveEnabledMods: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %v, want none", entries)
	}
}

func TestResolveEnabledModsUnknownIDErrors(t *testing.T) {
	mods := []mod.Mod{{ID: "known_mod"}}
	order := conflict.LoadOrder{"known_mod", "ghost_mod"}

	_, err := resolveEnabledMods(order, mods)
	if err == nil {
		t.Fatal("expected an error for a load-order entry not present in mods")
	}
	var unknown *UnknownModsError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v (%T), want *UnknownModsError", err, err)
	}
	if len(unknown.IDs) != 1 || unknown.IDs[0] != "ghost_mod" {
		t.Errorf("UnknownModsError.IDs = %v, want [ghost_mod]", unknown.IDs)
	}
}

func TestResolveEnabledModsDuplicateIDKeepsFirstOccurrence(t *testing.T) {
	mods := []mod.Mod{{ID: "mod_a"}, {ID: "mod_b"}}
	order := conflict.LoadOrder{"mod_a", "mod_b", "mod_a"}

	entries, err := resolveEnabledMods(order, mods)
	if err != nil {
		t.Fatalf("resolveEnabledMods: %v", err)
	}
	want := []string{"mod/mod_a.mod", "mod/mod_b.mod"}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %v, want %v", entries, want)
	}
}

func TestResolveEnabledModsForwardSlashRegardlessOfOS(t *testing.T) {
	mods := []mod.Mod{{ID: "ugc_1830063425"}, {ID: "pdx_00001"}, {ID: "my_local_mod"}}
	order := conflict.LoadOrder{"ugc_1830063425", "pdx_00001", "my_local_mod"}

	entries, err := resolveEnabledMods(order, mods)
	if err != nil {
		t.Fatalf("resolveEnabledMods: %v", err)
	}
	want := []string{"mod/ugc_1830063425.mod", "mod/pdx_00001.mod", "mod/my_local_mod.mod"}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries (GOOS=%s) = %v, want %v", runtime.GOOS, entries, want)
	}
	for _, e := range entries {
		if strings.Contains(e, `\`) {
			t.Errorf("entry %q contains a backslash on GOOS=%s - must always be forward-slashed", e, runtime.GOOS)
		}
	}
}

func TestWriteStateRequiresStateDir(t *testing.T) {
	// Intentionally no t.TempDir() anywhere in this test - proving no
	// directory is required OR created when StateDir is left unset.
	_, err := WriteState(conflict.LoadOrder{}, nil, testClassicGame(), Options{})
	if err != ErrStateDirRequired {
		t.Errorf("err = %v, want ErrStateDirRequired", err)
	}
}

func TestWriteStateRejectsUnsupportedDescriptorType(t *testing.T) {
	cfg := testClassicGame()
	cfg.DescriptorType = mod.DescriptorJSONv1

	_, err := WriteState(conflict.LoadOrder{}, nil, cfg, Options{StateDir: t.TempDir()})
	if err != ErrUnsupportedDescriptorType {
		t.Errorf("err = %v, want ErrUnsupportedDescriptorType", err)
	}
}
