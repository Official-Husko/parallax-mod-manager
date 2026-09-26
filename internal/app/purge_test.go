package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

func TestPurgeModsEmitsModsChangedWhenSomethingWasActuallyDeleted(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	cfg := game.Stellaris
	modDir := filepath.Join(data, "Paradox Interactive", "Stellaris", "mod")

	// A local mod with a stub but a content folder that's missing entirely -
	// FindEmptyMods'/PurgeMods' own definition of "empty", so this really
	// does get deleted below (not skipped as "has real content now").
	empty := filepath.Join(t.TempDir(), "does-not-exist")
	writeText(t, filepath.Join(modDir, "Empty Mod.mod"), "name=\"Empty Mod\"\npath=\""+empty+"\"\n")

	var mu sync.Mutex
	var events []string
	a := &App{
		ctx:          context.Background(),
		registry:     game.NewRegistry([]game.GameConfig{cfg}),
		preferences:  preferences.Defaults(),
		configAppDir: t.TempDir(),
		eventSink: func(name string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, name)
		},
	}

	result, err := a.PurgeMods(cfg.ID, []string{"Empty Mod"})
	if err != nil {
		t.Fatalf("PurgeMods: %v", err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != "Empty Mod" {
		t.Fatalf("result.Deleted = %+v, want exactly [\"Empty Mod\"]", result.Deleted)
	}
	if _, err := os.Stat(filepath.Join(modDir, "Empty Mod.mod")); !os.IsNotExist(err) {
		t.Errorf("stub file should have been removed, stat err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	saw := false
	for _, e := range events {
		if e == "mods-changed" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected \"mods-changed\" to be emitted, got events %+v - Editor (and anything else caching a mod list) would keep showing the deleted mod otherwise", events)
	}
}

func TestPurgeModsDoesNotEmitWhenNothingWasDeleted(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	cfg := game.Stellaris

	var mu sync.Mutex
	var events []string
	a := &App{
		ctx:          context.Background(),
		registry:     game.NewRegistry([]game.GameConfig{cfg}),
		preferences:  preferences.Defaults(),
		configAppDir: t.TempDir(),
		eventSink: func(name string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, name)
		},
	}

	// No such mod exists at all - PurgeMods reports it as an error, deletes
	// nothing, and must not emit a change nobody made.
	result, err := a.PurgeMods(cfg.ID, []string{"Nonexistent"})
	if err != nil {
		t.Fatalf("PurgeMods: %v", err)
	}
	if len(result.Deleted) != 0 || len(result.Errors) != 1 {
		t.Fatalf("result = %+v, want 0 deleted and 1 error", result)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 0 {
		t.Errorf("events = %+v, want none - nothing was actually deleted", events)
	}
}
