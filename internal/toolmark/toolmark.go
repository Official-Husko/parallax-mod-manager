// Package toolmark writes and keeps up to date PARALLAX_TOOLS.md, a small,
// plain-text marker this app leaves in a mod's own folder the moment one of
// its own tools actually writes real content into it - Editor Save,
// auto-translate (both destinations), GeneratePatch, Duplicate, New Mod.
// It exists purely so the mod's own folder says, in the open, what this app
// did to it: anyone looking at the folder later - the mod's own author, a
// collaborator, the person using this app themselves months on - can see
// which of this app's tools have touched it and when, without needing this
// app at all to find out. Nothing here is ever read back by this app
// itself (it is not a data file this app depends on), and nothing here is
// sent anywhere.
package toolmark

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/about"
	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
)

// FileName is the marker's own name, written at a mod's own root - next to
// its descriptor.mod, the same level a mod's own README or LICENSE would
// already sit.
const FileName = "PARALLAX_TOOLS.md"

const header = "# Made with Parallax Mod Manager\n\n" +
	"This mod was edited using [Parallax Mod Manager](" + about.RepoURL + ").\n\n" +
	"The following was done:\n\n"

// entryLine matches one already-written history line - "- 2026-09-25: Edited in the Editor".
var entryLine = regexp.MustCompile(`^- (\d{4}-\d{2}-\d{2}): (.+)$`)

// Record notes that action just happened to modDir: creates
// modDir/PARALLAX_TOOLS.md the first time anything is ever recorded for it,
// and otherwise adds one more line to its existing history - newest first.
// action is a short, plain-English sentence fragment describing what was
// actually done ("Edited in the Editor", "Translated into German using
// Translanova", "Generated as a load-order patch mod") - written exactly as
// given, never interpreted or embellished.
//
// Recording the exact same action again on the same day is a no-op, so a
// burst of saves in one sitting never spams the file with repeats; the same
// action recorded again on a later day is a new, real line - a genuine
// history of when this mod was touched, not just a "last used" snapshot
// that forgets everything before it.
//
// modDir or action empty is a no-op, never an error - there is nothing
// meaningful to record. Any real error here is only ever logged by the
// caller, never surfaced as the reason a real save failed - the same
// "never fails the real save" treatment this app's own version-bump
// snapshot already gets (see internal/modedit.WriteVersionSnapshot's own
// doc comment); this file is a courtesy note, not something anything here
// depends on existing.
func Record(modDir, action string) error {
	if modDir == "" || action == "" {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	path := filepath.Join(modDir, FileName)

	var entries []string // "YYYY-MM-DD: action", newest first
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		for _, line := range strings.Split(string(data), "\n") {
			m := entryLine.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if m[1] == today && m[2] == action {
				return nil // already recorded today, nothing to do
			}
			entries = append(entries, m[1]+": "+m[2])
		}
	case os.IsNotExist(err):
		// First time anything has ever been recorded for this mod - entries stays empty.
	default:
		return err
	}

	entries = append([]string{today + ": " + action}, entries...)

	var b strings.Builder
	b.WriteString(header)
	for _, e := range entries {
		b.WriteString("- " + e + "\n")
	}
	_, err = atomicfile.Write(modDir, FileName, []byte(b.String()))
	return err
}
