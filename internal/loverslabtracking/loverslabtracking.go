// Package loverslabtracking keeps a record of which locally installed mods came from
// LoversLab, one file per game - what the update check compares against later to
// notice a newer version. Mirrors internal/modnotes' own Store{Dir}/one-JSONC-file-
// per-game shape,
// since it's the same kind of small, per-mod, per-game side data - written as JSONC
// with a comment explaining the file, and hand-editable, even though (unlike a note)
// nothing here is normally typed by a person; it's recorded automatically by
// LoversLabInstall each time a mod is downloaded and installed.
package loverslabtracking

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Entry is what's tracked for one mod installed from LoversLab.
type Entry struct {
	// FileURL is the file's own page - what LoversLabFileDetail/LoversLabChangelog
	// and the update check re-fetch to see if anything changed.
	FileURL string `json:"fileUrl"`
	// FileID is the numeric id from the file's URL (loverslab.FileSummary.ID) -
	// kept alongside FileURL for display/debugging, not used to build any request
	// itself (FileURL already has everything needed for that).
	FileID int `json:"fileId"`
	// Title is the file's name when it was installed, only so a person reading this
	// file (or a "here's what might have an update" list) can tell entries apart;
	// the mod itself is found by its own ID (this entry's own map key).
	Title string `json:"title"`
	// ThumbnailURL is the file's own card image at install time (loverslab.FileSummary.
	// ThumbnailURL - always real for a file the listing page ever showed, never a mock or
	// a later re-fetch), so the Browse tab's own Installed section can show the same card
	// image the browsing grid does instead of a bare placeholder. Empty for anything
	// installed before this field existed, or for a file that had no screenshot at all.
	ThumbnailURL string `json:"thumbnailUrl"`
	// InstalledDateModified is the site's own "dateModified" ISO 8601 timestamp (see
	// loverslab.FileDetail) at install time - the update check re-fetches the file's
	// own page and compares this against its current value; a real timestamp, unlike
	// the category listing's own display string (FileSummary.Updated, e.g. "2 days
	// ago"), which the site renders inconsistently depending on age.
	InstalledDateModified string `json:"installedDateModified"`
	// InstalledAt is when this app installed or last updated this mod (unix
	// seconds) - shown alongside Title for a person's own reference; not read by
	// the update check itself.
	InstalledAt int64 `json:"installedAt"`
	// ArchiveName is the exact archive filename this mod was actually built from
	// (loverslab.FileDownload.Name) - a permanent record kept independent of
	// LoversLab's own Files list, which can later rename, replace, or drop that
	// same entry without this app losing track of what it originally installed
	// from. Never a working download link itself (LoversLabDownloadDialog's own
	// URLs are one-time and expire almost immediately) - for record-keeping and
	// display only. Empty for anything installed before this field existed.
	ArchiveName string `json:"archiveName"`
	// ArchivePosted is that same archive's own "posted"/release date, exactly as
	// LoversLab's download dialog displayed it (loverslab.FileDownload.Posted) -
	// the file's own real release date, distinct from InstalledAt (when this app
	// grabbed it) and from InstalledDateModified (the page's own, not this one
	// specific archive's own, last-modified timestamp).
	ArchivePosted string `json:"archivePosted"`
	// ContentDir is this mod's real content folder, as an informational record
	// only - never itself the source of truth for where a mod's content lives
	// (its own stub descriptor's path= is that - see locateLoversLabInstall in
	// internal/app/loverslabinstall.go), so a person reading this file by hand
	// can see where a mod actually is without cross-referencing its stub too.
	ContentDir string `json:"contentDir"`
}

// Store keeps LoversLab install tracking on disk, one file per game in Dir.
type Store struct {
	Dir string
}

func (s Store) path(gameID string) string {
	return filepath.Join(s.Dir, gameID+".jsonc")
}

// Load returns gameID's tracked installs by mod ID. A game with no file has none. A
// file that cannot be read or parsed is an error, never silently treated as empty -
// the next save would otherwise replace what was tracked with nothing.
func (s Store) Load(gameID string) (map[string]Entry, error) {
	if s.Dir == "" {
		return map[string]Entry{}, nil
	}
	data, err := os.ReadFile(s.path(gameID))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loverslabtracking: reading %s: %w", s.path(gameID), err)
	}
	var f struct {
		Installs map[string]Entry `json:"installs"`
	}
	if err := jsonc.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("loverslabtracking: %s is not valid (fix or remove it by hand; nothing was changed): %w", s.path(gameID), err)
	}
	installs := make(map[string]Entry, len(f.Installs))
	for id, e := range f.Installs {
		if strings.TrimSpace(e.FileURL) != "" {
			installs[id] = e
		}
	}
	return installs, nil
}

// Save writes gameID's tracked installs atomically, replacing the file.
func (s Store) Save(gameID string, installs map[string]Entry) error {
	if s.Dir == "" {
		return errors.New("loverslabtracking: there is no settings folder to track installs in")
	}
	if _, err := atomicfile.Write(s.Dir, filepath.Base(s.path(gameID)), render(installs)); err != nil {
		return fmt.Errorf("loverslabtracking: saving installs for %q: %w", gameID, err)
	}
	return nil
}

// With returns installs with modID's entry set to e (a zero-value Entry removes it -
// e.g. once a mod is uninstalled/purged). installs itself is not modified.
func With(installs map[string]Entry, modID string, e Entry) (map[string]Entry, error) {
	if strings.TrimSpace(modID) == "" {
		return nil, errors.New("loverslabtracking: an entry needs a mod")
	}
	out := make(map[string]Entry, len(installs)+1)
	for id, existing := range installs {
		out[id] = existing
	}
	if strings.TrimSpace(e.FileURL) == "" {
		delete(out, modID)
		return out, nil
	}
	out[modID] = e
	return out, nil
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// render writes the file: a comment saying what it is, then one entry per line in ID
// order so the file stays easy to read and diff.
func render(installs map[string]Entry) []byte {
	ids := make([]string, 0, len(installs))
	for id := range installs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	b.WriteString(`// Parallax Mod Manager - which of this game's mods were installed from LoversLab.
//
// One entry per mod, keyed by the mod's ID (its descriptor filename without the
// extension - see internal/mod's LoversLabFilePrefix). Recorded automatically each
// time a mod is downloaded and installed from Browse; "installedDateModified" is what
// the update check compares against LoversLab's own current value for the same file
// to notice a newer version. "archiveName"/"archivePosted"/"contentDir" are a
// permanent record of what was actually downloaded and where it went, kept
// independent of LoversLab's own Files list (which can later rename, replace, or
// drop that same entry) and independent of the mod's own stub descriptor (which is
// still the real source of truth for where its content lives) - so this file alone
// still says what was installed and from where, even if either of those changes out
// from under it later. Deleting an entry here just stops that one mod being checked
// for updates - it does not uninstall anything.
{
  "installs": {
`)
	for i, id := range ids {
		e := installs[id]
		b.WriteString("    " + quote(id) + ": {" +
			"\"fileUrl\": " + quote(e.FileURL) + ", " +
			"\"fileId\": " + fmt.Sprint(e.FileID) + ", " +
			"\"title\": " + quote(e.Title) + ", " +
			"\"thumbnailUrl\": " + quote(e.ThumbnailURL) + ", " +
			"\"installedDateModified\": " + quote(e.InstalledDateModified) + ", " +
			"\"installedAt\": " + fmt.Sprint(e.InstalledAt) + ", " +
			"\"archiveName\": " + quote(e.ArchiveName) + ", " +
			"\"archivePosted\": " + quote(e.ArchivePosted) + ", " +
			"\"contentDir\": " + quote(e.ContentDir) +
			"}")
		if i < len(ids)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  }\n}\n")
	return []byte(b.String())
}
