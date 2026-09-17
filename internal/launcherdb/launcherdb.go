// Package launcherdb reads the Paradox Launcher's own launcher-v2.sqlite
// database, read-only - see docs/launcher-database.md. Its schema is
// confirmed against two real installs on the same machine (Stellaris,
// Hearts of Iron IV), not just a secondary source.
//
// This project never writes to this file: the launcher owns it, can
// rewrite it out from under this app at any time, and nothing this
// project needs to do to activate mods or launch the game depends on it
// existing at all (see docs/game-launching.md - internal/launch writes
// dlc_load.json/mods_registry.json/game_data.json directly from this
// project's own scan and playset storage). The one thing it's genuinely
// useful for is a one-shot, read-only import of a user's existing
// Launcher playsets, so switching to this app doesn't mean manually
// rebuilding a load order by hand.
package launcherdb

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Filename is launcher-v2.sqlite's own real filename, confirmed to live
// directly under a classic-descriptor game's UserDataDir - the same
// directory as dlc_load.json/mods_registry.json/game_data.json.
const Filename = "launcher-v2.sqlite"

// ErrNotFound means dir has no launcher-v2.sqlite - not itself unusual (a
// fresh install, or a game the real Paradox Launcher has never actually
// been opened against), just "nothing to import here."
var ErrNotFound = errors.New("launcherdb: no launcher-v2.sqlite found")

// Playset is one playset (a Launcher-native named, ordered mod collection)
// found in launcher-v2.sqlite.
type Playset struct {
	ID       string
	Name     string
	IsActive bool
	Mods     []PlaysetMod // real load-order position, ascending
}

// PlaysetMod is one mod entry within a Playset. GameRegistryID is exactly
// the "mod/<id>.mod" string dlc_load.json's own enabled_mods and this
// project's mods_registry.json both use (see internal/launch) - the same
// bridge value, so an imported playset's mods can be matched straight
// against this project's own scanned mod list, with no extra resolution
// step beyond stripping that same "mod/"..".mod" wrapping back off.
type PlaysetMod struct {
	GameRegistryID string
	DisplayName    string
	Enabled        bool
}

// ListPlaysets opens dir's real launcher-v2.sqlite read-only - both at the
// SQLite URI level (mode=ro, immutable: this may be a database the real
// Launcher still has open) and at the SQL level (_query_only, rejecting a
// write even if the URI-level flags were somehow bypassed) - and returns
// every non-removed playset it holds, each with its own mods in real
// load-order position. Returns ErrNotFound, not a generic error, when dir
// simply has no such file.
func ListPlaysets(dir string) ([]Playset, error) {
	path := filepath.Join(dir, Filename)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("launcherdb: %w", err)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1&_query_only=1")
	if err != nil {
		return nil, fmt.Errorf("launcherdb: opening %s: %w", path, err)
	}
	defer db.Close()

	playsets, err := queryPlaysets(db)
	if err != nil {
		return nil, fmt.Errorf("launcherdb: %w", err)
	}
	for i := range playsets {
		mods, err := queryPlaysetMods(db, playsets[i].ID)
		if err != nil {
			return nil, fmt.Errorf("launcherdb: %w", err)
		}
		playsets[i].Mods = mods
	}
	return playsets, nil
}

func queryPlaysets(db *sql.DB) ([]Playset, error) {
	rows, err := db.Query(`SELECT id, name, isActive FROM playsets WHERE isRemoved = 0 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var playsets []Playset
	for rows.Next() {
		var p Playset
		var isActive sql.NullBool
		if err := rows.Scan(&p.ID, &p.Name, &isActive); err != nil {
			return nil, err
		}
		p.IsActive = isActive.Bool
		playsets = append(playsets, p)
	}
	return playsets, rows.Err()
}

func queryPlaysetMods(db *sql.DB, playsetID string) ([]PlaysetMod, error) {
	rows, err := db.Query(`
		SELECT m.gameRegistryId, m.steamId, m.displayName, m.name, pm.enabled
		FROM playsets_mods pm
		JOIN mods m ON m.id = pm.modId
		WHERE pm.playsetId = ?
		ORDER BY pm.position
	`, playsetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mods []PlaysetMod
	for rows.Next() {
		var gameRegistryID, steamID, displayName, name sql.NullString
		var enabled sql.NullBool
		if err := rows.Scan(&gameRegistryID, &steamID, &displayName, &name, &enabled); err != nil {
			return nil, err
		}
		id := registryID(gameRegistryID.String, steamID.String)
		if id == "" {
			continue // no usable identifier - nothing this project's own scan could ever match
		}
		label := displayName.String
		if label == "" {
			label = name.String
		}
		if label == "" {
			label = id
		}
		mods = append(mods, PlaysetMod{
			GameRegistryID: id,
			DisplayName:    label,
			Enabled:        enabled.Bool,
		})
	}
	return mods, rows.Err()
}

// registryID mirrors a real fallback confirmed in a second reference
// tool's own production code: a row missing gameRegistryId can still be
// resolved from its steamId, matching Steam Workshop's own
// "mod/ugc_<id>.mod" filename convention (see docs/mod-sources.md).
// Backslashes are normalized to forward slashes defensively - this
// project's own confirmed real data always uses forward slashes, but a
// Windows-written entry isn't something to assume about sight unseen.
func registryID(gameRegistryID, steamID string) string {
	if gameRegistryID != "" {
		return strings.ReplaceAll(gameRegistryID, `\`, "/")
	}
	if steamID != "" {
		return "mod/ugc_" + steamID + ".mod"
	}
	return ""
}
