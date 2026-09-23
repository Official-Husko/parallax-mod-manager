package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/modnotes"
)

func notesApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "mod_notes")
	cfg := game.Stellaris
	return &App{registry: game.NewRegistry([]game.GameConfig{cfg}), modNotes: modnotes.Store{Dir: dir}}, cfg.ID
}

func TestNotesCanBeSavedChangedAndRemoved(t *testing.T) {
	a, g := notesApp(t)
	if notes, err := a.ModNotes(g); err != nil || len(notes) != 0 {
		t.Fatalf("fresh: %v, %v", notes, err)
	}
	if err := a.SetModNote(g, "ugc_1", "Flat Nameplates", "crashes with X\r\nuntil updated"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetModNote(g, "Lustful Void", "Lustful Void", "local copy"); err != nil {
		t.Fatal(err)
	}
	notes, err := a.ModNotes(g)
	if err != nil || notes["ugc_1"] != "crashes with X\nuntil updated" || notes["Lustful Void"] != "local copy" {
		t.Fatalf("notes = %v, %v", notes, err)
	}
	if err := a.SetModNote(g, "ugc_1", "Flat Nameplates", "  "); err != nil {
		t.Fatal(err)
	}
	notes, _ = a.ModNotes(g)
	if _, ok := notes["ugc_1"]; ok || len(notes) != 1 {
		t.Errorf("an empty note should remove it: %v", notes)
	}
}

func TestANoteIsNeverWrittenToTheLog(t *testing.T) {
	applog.Default().Clear()
	a, g := notesApp(t)
	secret := "my private thoughts about this mod"
	if err := a.SetModNote(g, "ugc_1", "Some Mod", secret); err != nil {
		t.Fatal(err)
	}
	logged := false
	for _, e := range applog.Default().Entries() {
		if strings.Contains(e.Message, secret) {
			t.Errorf("the note text is in the log: %s", e.Message)
		}
		if e.Component == "Notes" && strings.Contains(e.Message, "Some Mod") {
			logged = true
		}
	}
	if !logged {
		t.Error("saving a note should leave a line in the log")
	}
	applog.Default().Clear()
}

func TestABrokenNotesFileIsNotOverwrittenBySaving(t *testing.T) {
	a, g := notesApp(t)
	if err := os.MkdirAll(a.modNotes.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.modNotes.Dir, g+".jsonc")
	broken := `{"notes": {"a": {"text": "precious"`
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ModNotes(g); err == nil {
		t.Error("expected an error reading a broken file")
	}
	if err := a.SetModNote(g, "b", "B", "new"); err == nil {
		t.Error("expected an error saving over a broken file")
	}
	if data, _ := os.ReadFile(path); string(data) != broken {
		t.Errorf("the broken file was changed: %q", data)
	}
}

func TestNotesRefuseAnUnknownGameAndAnOverlongNote(t *testing.T) {
	a, g := notesApp(t)
	if _, err := a.ModNotes("nope"); err == nil {
		t.Error("unknown game")
	}
	if err := a.SetModNote("nope", "a", "A", "x"); err == nil {
		t.Error("unknown game")
	}
	if err := a.SetModNote(g, "a", "A", strings.Repeat("x", modnotes.MaxLength+1)); err == nil {
		t.Error("overlong note")
	}
	if notes, _ := a.ModNotes(g); len(notes) != 0 {
		t.Errorf("a refused note was saved: %v", notes)
	}
}
