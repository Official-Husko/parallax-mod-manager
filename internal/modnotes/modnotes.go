// Package modnotes keeps the notes a person writes about individual mods ("crashes with X",
// "waiting for the author to update", "needed for the Y playset"), one file per game.
//
// The notes are the person's own writing, so unlike the app's re-derivable caches a file that
// cannot be read is an error, never silently treated as empty: the next save would otherwise
// replace what the person wrote with nothing. Written as JSONC with a comment explaining the
// file, and hand-editable.
package modnotes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// MaxLength is the longest a note can be, in characters: plenty for a note, and a bound on
// how large one file can grow.
const MaxLength = 10000

// ErrTooLong means a note is over MaxLength.
var ErrTooLong = fmt.Errorf("modnotes: a note can be at most %d characters", MaxLength)

// Entry is the note for one mod.
type Entry struct {
	// Name is the mod's name when the note was written, only so a person reading the file
	// can tell entries apart; the mod is found by its ID.
	Name string `json:"name"`
	// Text is the note.
	Text string `json:"text"`
}

// Store keeps notes on disk, one file per game in Dir.
type Store struct {
	Dir string
}

func (s Store) path(gameID string) string {
	return filepath.Join(s.Dir, gameID+".jsonc")
}

// Load returns gameID's notes by mod ID. A game with no file has none. A file that cannot be
// read or parsed is an error (see the package comment).
func (s Store) Load(gameID string) (map[string]Entry, error) {
	if s.Dir == "" {
		return map[string]Entry{}, nil
	}
	data, err := os.ReadFile(s.path(gameID))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("modnotes: reading %s: %w", s.path(gameID), err)
	}
	var f struct {
		Notes map[string]Entry `json:"notes"`
	}
	if err := jsonc.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("modnotes: %s is not valid (fix or remove it by hand; nothing was changed): %w", s.path(gameID), err)
	}
	notes := make(map[string]Entry, len(f.Notes))
	for id, e := range f.Notes {
		if strings.TrimSpace(e.Text) != "" {
			notes[id] = e
		}
	}
	return notes, nil
}

// Save writes gameID's notes atomically, replacing the file.
func (s Store) Save(gameID string, notes map[string]Entry) error {
	if s.Dir == "" {
		return errors.New("modnotes: there is no settings folder to keep notes in")
	}
	if _, err := atomicfile.Write(s.Dir, filepath.Base(s.path(gameID)), render(notes)); err != nil {
		return fmt.Errorf("modnotes: saving notes for %q: %w", gameID, err)
	}
	return nil
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// render writes the file: a comment saying what it is, then one entry per line in ID order so
// the file stays easy to read and diff.
func render(notes map[string]Entry) []byte {
	ids := make([]string, 0, len(notes))
	for id := range notes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	b.WriteString(`// Parallax Mod Manager - your notes about this game's mods.
//
// One entry per mod, keyed by the mod's ID: the name of its .mod file without the extension
// (for a Steam Workshop mod, ugc_ and its Workshop number). "name" only helps you tell the
// entries apart when reading this file; "text" is the note. Newlines in a note are written as \n.
//
// You normally write notes in the app: select a mod and use the Notes box on its Overview tab,
// or right-click it and choose Edit note. Changes made here are picked up the next time the
// app starts. Comments in this file are not kept when the app saves it.
{
  "notes": {
`)
	for i, id := range ids {
		e := notes[id]
		b.WriteString("    " + quote(id) + ": {\"name\": " + quote(e.Name) + ", \"text\": " + quote(e.Text) + "}")
		if i < len(ids)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }\n}\n")
	return []byte(b.String())
}

// Clean tidies a note as typed: line endings made "\n", blank space around it removed. It
// reports ErrTooLong for one over MaxLength.
func Clean(text string) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))
	if utf8.RuneCountInString(text) > MaxLength {
		return "", ErrTooLong
	}
	return text, nil
}

// With returns notes with modID's note set to text (an empty text removes it). notes itself is
// not modified.
func With(notes map[string]Entry, modID, name, text string) (map[string]Entry, error) {
	if strings.TrimSpace(modID) == "" {
		return nil, errors.New("modnotes: a note needs a mod")
	}
	text, err := Clean(text)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Entry, len(notes)+1)
	for id, e := range notes {
		out[id] = e
	}
	if text == "" {
		delete(out, modID)
		return out, nil
	}
	out[modID] = Entry{Name: strings.TrimSpace(name), Text: text}
	return out, nil
}
