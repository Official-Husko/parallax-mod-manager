package app

import (
	"fmt"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/modnotes"
)

// ModNotes returns the notes written about gameID's mods, by mod ID. A notes file that cannot
// be read is an error (the person's writing is never treated as empty and then replaced).
func (a *App) ModNotes(gameID string) (map[string]string, error) {
	if _, ok := a.registry.Get(gameID); !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	a.modNotesMu.Lock()
	entries, err := a.modNotes.Load(gameID)
	a.modNotesMu.Unlock()
	if err != nil {
		applog.For("Notes").Errorf("%v", err)
		return nil, err
	}
	notes := make(map[string]string, len(entries))
	for id, e := range entries {
		notes[id] = e.Text
	}
	return notes, nil
}

// SetModNote saves the note for one of gameID's mods; an empty note removes it. modName is kept
// beside the note in the file only so a person reading it can tell entries apart. The note's
// text is never written to the activity log.
func (a *App) SetModNote(gameID, modID, modName, text string) error {
	if _, ok := a.registry.Get(gameID); !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	a.modNotesMu.Lock()
	defer a.modNotesMu.Unlock()

	current, err := a.modNotes.Load(gameID)
	if err != nil {
		applog.For("Notes").Errorf("%v", err)
		return err
	}
	next, err := modnotes.With(current, modID, modName, text)
	if err != nil {
		return err
	}
	if err := a.modNotes.Save(gameID, next); err != nil {
		applog.For("Notes").Errorf("%v", err)
		return err
	}
	if note, ok := next[modID]; ok {
		applog.For("Notes").Infof("saved a note for '%s' in '%s' (%d characters)", noteLabel(modID, modName), a.gameLabel(gameID), len([]rune(note.Text)))
	} else if _, had := current[modID]; had {
		applog.For("Notes").Infof("removed the note for '%s' in '%s'", noteLabel(modID, modName), a.gameLabel(gameID))
	}
	return nil
}

func noteLabel(modID, modName string) string {
	if modName != "" {
		return modName
	}
	return modID
}
