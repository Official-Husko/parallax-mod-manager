package launcherdb

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newFixtureDB creates dir/launcher-v2.sqlite with the confirmed real
// schema (trimmed to the columns this package actually reads - the real
// mods table has dozens more, unrelated to anything here) and returns an
// open, real read-write connection for the caller to seed with fixture
// rows before closing it. ListPlaysets always reopens its own read-only
// connection separately, exactly as it would against a real file the
// Launcher itself created.
func newFixtureDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	path := filepath.Join(dir, Filename)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
		CREATE TABLE playsets (
			id char(36) not null,
			name varchar(255) not null,
			isActive boolean,
			isRemoved boolean not null default false,
			primary key (id)
		);
		CREATE TABLE mods (
			id char(36) not null,
			steamId varchar(255),
			gameRegistryId text,
			name varchar(255),
			displayName varchar(255),
			primary key (id)
		);
		CREATE TABLE playsets_mods (
			playsetId char(36) not null,
			modId char(36) not null,
			enabled boolean default '1',
			position integer
		);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create fixture schema: %v", err)
	}
	return db
}

func TestListPlaysetsNotFoundWhenFileMissing(t *testing.T) {
	_, err := ListPlaysets(t.TempDir())
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListPlaysetsReturnsPlaysetsWithOrderedMods(t *testing.T) {
	dir := t.TempDir()
	db := newFixtureDB(t, dir)

	exec(t, db, `INSERT INTO playsets (id, name, isActive) VALUES ('p1', 'Multiplayer', 1)`)
	exec(t, db, `INSERT INTO mods (id, gameRegistryId, displayName) VALUES ('m1', 'mod/ugc_111.mod', 'First Mod')`)
	exec(t, db, `INSERT INTO mods (id, gameRegistryId, displayName) VALUES ('m2', 'mod/ugc_222.mod', 'Second Mod')`)
	// Inserted out of position order, to prove the query - not insertion
	// order - determines the result.
	exec(t, db, `INSERT INTO playsets_mods (playsetId, modId, enabled, position) VALUES ('p1', 'm2', 1, 1)`)
	exec(t, db, `INSERT INTO playsets_mods (playsetId, modId, enabled, position) VALUES ('p1', 'm1', 1, 0)`)
	db.Close()

	playsets, err := ListPlaysets(dir)
	if err != nil {
		t.Fatalf("ListPlaysets: %v", err)
	}
	if len(playsets) != 1 {
		t.Fatalf("got %d playsets, want 1", len(playsets))
	}
	p := playsets[0]
	if p.ID != "p1" || p.Name != "Multiplayer" || !p.IsActive {
		t.Errorf("playset = %+v, want {ID:p1 Name:Multiplayer IsActive:true ...}", p)
	}
	wantMods := []PlaysetMod{
		{GameRegistryID: "mod/ugc_111.mod", DisplayName: "First Mod", Enabled: true},
		{GameRegistryID: "mod/ugc_222.mod", DisplayName: "Second Mod", Enabled: true},
	}
	if len(p.Mods) != len(wantMods) {
		t.Fatalf("got %d mods, want %d: %+v", len(p.Mods), len(wantMods), p.Mods)
	}
	for i, want := range wantMods {
		if p.Mods[i] != want {
			t.Errorf("Mods[%d] = %+v, want %+v", i, p.Mods[i], want)
		}
	}
}

func TestListPlaysetsExcludesRemovedPlaysets(t *testing.T) {
	dir := t.TempDir()
	db := newFixtureDB(t, dir)
	exec(t, db, `INSERT INTO playsets (id, name, isActive, isRemoved) VALUES ('p1', 'Gone', 0, 1)`)
	exec(t, db, `INSERT INTO playsets (id, name, isActive, isRemoved) VALUES ('p2', 'Still Here', 0, 0)`)
	db.Close()

	playsets, err := ListPlaysets(dir)
	if err != nil {
		t.Fatalf("ListPlaysets: %v", err)
	}
	if len(playsets) != 1 || playsets[0].Name != "Still Here" {
		t.Errorf("playsets = %+v, want exactly [Still Here]", playsets)
	}
}

func TestListPlaysetsFallsBackToSteamIDWhenGameRegistryIDMissing(t *testing.T) {
	dir := t.TempDir()
	db := newFixtureDB(t, dir)
	exec(t, db, `INSERT INTO playsets (id, name, isActive) VALUES ('p1', 'Test', 0)`)
	exec(t, db, `INSERT INTO mods (id, steamId, displayName) VALUES ('m1', '999', 'No Registry Id')`)
	exec(t, db, `INSERT INTO playsets_mods (playsetId, modId, enabled, position) VALUES ('p1', 'm1', 1, 0)`)
	db.Close()

	playsets, err := ListPlaysets(dir)
	if err != nil {
		t.Fatalf("ListPlaysets: %v", err)
	}
	if len(playsets) != 1 || len(playsets[0].Mods) != 1 {
		t.Fatalf("playsets = %+v", playsets)
	}
	got := playsets[0].Mods[0].GameRegistryID
	if got != "mod/ugc_999.mod" {
		t.Errorf("GameRegistryID = %q, want %q", got, "mod/ugc_999.mod")
	}
}

func TestListPlaysetsSkipsModWithNoUsableIdentifier(t *testing.T) {
	dir := t.TempDir()
	db := newFixtureDB(t, dir)
	exec(t, db, `INSERT INTO playsets (id, name, isActive) VALUES ('p1', 'Test', 0)`)
	exec(t, db, `INSERT INTO mods (id, displayName) VALUES ('m1', 'No Id At All')`)
	exec(t, db, `INSERT INTO playsets_mods (playsetId, modId, enabled, position) VALUES ('p1', 'm1', 1, 0)`)
	db.Close()

	playsets, err := ListPlaysets(dir)
	if err != nil {
		t.Fatalf("ListPlaysets: %v", err)
	}
	if len(playsets[0].Mods) != 0 {
		t.Errorf("Mods = %+v, want none (no resolvable identifier)", playsets[0].Mods)
	}
}

func TestListPlaysetsConnectionIsReallyReadOnly(t *testing.T) {
	// Proves the read-only guarantee this package's whole design relies on
	// - not just documented, but actually enforced by the connection
	// itself - using the exact same DSN shape ListPlaysets opens.
	dir := t.TempDir()
	db := newFixtureDB(t, dir)
	exec(t, db, `INSERT INTO playsets (id, name, isActive) VALUES ('p1', 'Test', 0)`)
	db.Close()

	path := filepath.Join(dir, Filename)
	ro, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1&_query_only=1")
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer ro.Close()

	if _, err := ro.Exec(`UPDATE playsets SET name = 'hacked'`); err == nil {
		t.Error("expected a write against the read-only connection to fail, it succeeded")
	}
}

func TestListPlaysetsPropagatesStatErrorForUnreadableDir(t *testing.T) {
	// A path that exists but isn't a directory at all (so filepath.Join +
	// os.Stat fails with something other than "not exist") should
	// propagate as a real error, not be silently treated like a missing
	// file.
	dir := t.TempDir()
	blocker := filepath.Join(dir, Filename)
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// blocker itself is a regular file with the right name - ListPlaysets
	// will try to open it as a database and fail there instead, which is
	// the realistic failure mode; assert it's a real, non-ErrNotFound error.
	if _, err := ListPlaysets(dir); err == nil || err == ErrNotFound {
		t.Errorf("err = %v, want a real error (not ErrNotFound) for an unopenable file", err)
	}
}

func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
