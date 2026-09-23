package app

import (
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// patchLines returns the "Patch" lines the app has logged so far.
func patchLines() []applog.Entry {
	var out []applog.Entry
	for _, e := range applog.Default().Entries() {
		if e.Component == "Patch" {
			out = append(out, e)
		}
	}
	return out
}

func TestLogPatchStateRepeatsNothingUntilTheStateChanges(t *testing.T) {
	applog.Default().Clear()
	var a App

	stale := library.PatchSummary{Exists: true, Generation: 2, Changed: 3, New: 1}
	for i := 0; i < 5; i++ {
		a.logPatchState("stellaris", stale)
	}
	lines := patchLines()
	if len(lines) != 1 {
		t.Fatalf("the same out-of-date state logged %d times over 5 scans, want once: %+v", len(lines), lines)
	}
	if lines[0].Level != applog.Warn.String() || !strings.Contains(lines[0].Message, "3 changed, 1 new") {
		t.Errorf("first line = %+v, want a warning with the counts", lines[0])
	}

	// A different state is news.
	a.logPatchState("stellaris", library.PatchSummary{Exists: true, Generation: 2, Changed: 4, New: 1})
	if got := len(patchLines()); got != 2 {
		t.Fatalf("a changed state should log again, have %d lines", got)
	}

	// Another game's identical state is its own news.
	a.logPatchState("hoi4", library.PatchSummary{Exists: true, Generation: 2, Changed: 4, New: 1})
	if got := len(patchLines()); got != 3 {
		t.Fatalf("a second game's status should log on its own, have %d lines", got)
	}

	// Up to date is a plain info line, once.
	current := library.PatchSummary{Exists: true, Generation: 3, Patched: 10}
	a.logPatchState("stellaris", current)
	a.logPatchState("stellaris", current)
	lines = patchLines()
	if len(lines) != 4 || lines[3].Level != applog.Info.String() || !strings.Contains(lines[3].Message, "up to date") {
		t.Fatalf("expected one info line for the up-to-date state, got %+v", lines)
	}
}

func TestLogPatchStateForgetsAGameWhosePatchIsGone(t *testing.T) {
	applog.Default().Clear()
	var a App

	stale := library.PatchSummary{Exists: true, Generation: 1, Obsolete: 2}
	a.logPatchState("stellaris", stale)
	a.logPatchState("stellaris", library.PatchSummary{}) // the patch was deleted
	a.logPatchState("stellaris", stale)                  // ...and a new one is stale in the same way
	if got := len(patchLines()); got != 2 {
		t.Fatalf("a patch that came back must be reported afresh, have %d lines", got)
	}
}

func TestSavingPreferencesOnlyRestartsTheWatcherWhenWatchingIsToggled(t *testing.T) {
	// The registry knows no games, so restarting the watcher for the game being
	// watched fails with "unknown game" - which makes a restart observable
	// without touching the filesystem.
	a := App{registry: game.NewRegistry(nil), watchedGameID: "stellaris"}
	a.preferences = preferences.Defaults()
	a.preferences.ScanForNewMods = false

	p := a.preferences
	p.LastSelectedGame = "stellaris"
	p.CloseAfterLaunch = !p.CloseAfterLaunch
	if err := a.savePreferences(p, true); err != nil {
		t.Fatalf("a save that leaves scanning alone must not touch the watcher: %v", err)
	}

	p.ScanForNewMods = true
	if err := a.savePreferences(p, true); err == nil {
		t.Fatal("turning scanning on must restart the watcher")
	}

	// The same value again is not a toggle.
	if err := a.savePreferences(p, true); err != nil {
		t.Fatalf("saving the same scanning setting again must not restart the watcher: %v", err)
	}

	p.ScanForNewMods = false
	if err := a.savePreferences(p, true); err != nil {
		t.Fatalf("turning scanning off just stops the watcher: %v", err)
	}
}
