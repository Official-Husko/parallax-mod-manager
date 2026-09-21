package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/backup"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

const backupTestGame = "test-game"

type backupEvents struct {
	mu           sync.Mutex
	results      []BackupResult
	changed      int
	other        []string
	limitNotices []BackupLimitNotice
}

func (e *backupEvents) sink(name string, data ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch name {
	case "backup-done":
		e.results = append(e.results, data[0].(BackupResult))
	case "backups-changed":
		e.changed++
	case "backup-limit":
		e.limitNotices = append(e.limitNotices, data[0].(BackupLimitNotice))
	default:
		e.other = append(e.other, name)
	}
}

func (e *backupEvents) snapshot() ([]BackupResult, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]BackupResult(nil), e.results...), e.changed
}

// backupApp is an App with one game whose mod folder is a temp folder, backups going to
// a temp folder and its settings kept in a temp config folder.
func backupApp(t *testing.T) (a *App, modDir, backupRoot, configDir string, events *backupEvents) {
	t.Helper()
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	modDir = filepath.Join(dataHome, "Paradox Interactive", "TestGame", "mod")
	cfg := game.GameConfig{ID: backupTestGame, DisplayName: "Test Game", FolderName: "TestGame", DescriptorType: mod.DescriptorClassic, ScanFolders: []string{"common"}}
	events = &backupEvents{}
	a = &App{ctx: context.Background(), registry: game.NewRegistry([]game.GameConfig{cfg}), eventSink: events.sink}
	configDir = t.TempDir()
	a.initBackups(configDir)
	t.Cleanup(a.stopBackups)
	backupRoot = filepath.Join(t.TempDir(), "backups")
	if _, err := a.SetBackupFolder(backupRoot); err != nil {
		t.Fatal(err)
	}
	// The tests' temp drive may be small: the free-space guard is off unless a test asks for it.
	if _, err := a.SetBackupLimits(false, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	events.mu.Lock()
	events.changed = 0 // choosing the folder announced itself; the tests count what copies announce
	events.mu.Unlock()
	return a, modDir, backupRoot, configDir, events
}

// writeWorkshopStub creates an installed Workshop mod: its stub with a remote file id
// and a content folder holding a couple of files.
func writeWorkshopStub(t *testing.T, modDir, itemID, name string) string {
	t.Helper()
	id := "ugc_" + itemID
	put := func(rel, content string) {
		full := filepath.Join(modDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(id+".mod", "name = \""+name+"\"\npath = \""+id+"\"\nversion = \"1.0\"\nremote_file_id = \""+itemID+"\"\n")
	put(filepath.Join(id, "descriptor.mod"), "name = \""+name+"\"\n")
	put(filepath.Join(id, "common", "x.txt"), "thing = { id = "+itemID+" }\n")
	return filepath.Join(modDir, id)
}

func waitForBackups(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		a.backup.mu.Lock()
		busy := len(a.backup.running) > 0
		a.backup.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("backups did not finish")
}

func av(id, state, reason string) WorkshopAvailability {
	return WorkshopAvailability{RemoteFileID: id, State: state, Reason: reason}
}

func haveBackup(root, id string) bool {
	_, err := os.Stat(backup.FolderFor(root, backupTestGame, id))
	return err == nil
}

func TestAtRiskModeBacksUpDeletedAndPrivateModsOnly(t *testing.T) {
	a, modDir, root, _, events := backupApp(t)
	for id, name := range map[string]string{"111": "Deleted One", "222": "Private One", "333": "Unlisted One", "444": "Public One"} {
		writeWorkshopStub(t, modDir, id, name)
	}
	states := []WorkshopAvailability{av("111", "deleted", "page-gone"), av("222", "private", "record"), av("333", "unlisted", "record")}

	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)

	if !haveBackup(root, "111") || !haveBackup(root, "222") {
		t.Fatal("a deleted or private mod was not backed up")
	}
	if haveBackup(root, "333") || haveBackup(root, "444") {
		t.Error("an unlisted or ordinary mod was backed up in at-risk mode")
	}
	entries, _ := a.ListBackups(backupTestGame)
	reasons := map[string]string{}
	for _, e := range entries {
		reasons[e.RemoteFileID] = e.Reason
	}
	if reasons["111"] != backup.ReasonDeleted || reasons["222"] != backup.ReasonPrivate {
		t.Errorf("reasons = %v", reasons)
	}
	results, changed := events.snapshot()
	if len(results) != 2 || changed != 1 {
		t.Errorf("events: %d results, %d backups-changed, want 2 and 1", len(results), changed)
	}
	for _, r := range results {
		if r.Status != "ok" || r.Name == "" || r.Size == 0 {
			t.Errorf("result = %+v", r)
		}
	}

	// The 1:1 copy is the mod's own folder.
	data, err := os.ReadFile(filepath.Join(backup.FolderFor(root, backupTestGame, "111"), "common", "x.txt"))
	if err != nil || !strings.Contains(string(data), "id = 111") {
		t.Errorf("copy content = %q, %v", data, err)
	}
}

func TestSecondCheckOfUnchangedModsCopiesNothingAndSaysNothing(t *testing.T) {
	a, modDir, _, _, events := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Deleted One")
	states := []WorkshopAvailability{av("111", "deleted", "record")}
	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	before, changedBefore := events.snapshot()

	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	after, changedAfter := events.snapshot()
	if len(after) != len(before) || changedAfter != changedBefore {
		t.Errorf("a repeat check produced events: %d results, %d changed (was %d, %d)", len(after), changedAfter, len(before), changedBefore)
	}
}

func TestOffModeBacksNothingUp(t *testing.T) {
	a, modDir, root, _, events := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Deleted One")
	if _, err := a.SetBackupMode("off"); err != nil {
		t.Fatal(err)
	}
	a.preserveWorkshopMods(backupTestGame, []WorkshopAvailability{av("111", "deleted", "record")})
	waitForBackups(t, a)
	if haveBackup(root, "111") {
		t.Error("off mode made a backup")
	}
	if results, _ := events.snapshot(); len(results) != 0 {
		t.Errorf("off mode reported %d results", len(results))
	}
	states := []WorkshopAvailability{av("111", "deleted", "record")}
	a.attachBackupState(backupTestGame, states)
	if states[0].BackupState != "off" {
		t.Errorf("BackupState = %q, want off", states[0].BackupState)
	}
}

func TestAllModeBacksUpEveryWorkshopMod(t *testing.T) {
	a, modDir, root, _, _ := backupApp(t)
	for _, id := range []string{"111", "222", "333"} {
		writeWorkshopStub(t, modDir, id, "Mod "+id)
	}
	// A local mod is not a Workshop mod and is never copied.
	if err := os.MkdirAll(filepath.Join(modDir, "local_mod", "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(modDir, "local_mod.mod"), []byte("name = \"Local\"\npath = \"local_mod\"\n"), 0o644)

	if _, err := a.SetBackupMode("all"); err != nil {
		t.Fatal(err)
	}
	a.preserveWorkshopMods(backupTestGame, nil)
	waitForBackups(t, a)
	for _, id := range []string{"111", "222", "333"} {
		if !haveBackup(root, id) {
			t.Errorf("every-mod mode did not back up %s", id)
		}
	}
	entries, _ := a.ListBackups(backupTestGame)
	if len(entries) != 3 || entries[0].Reason != backup.ReasonAll {
		t.Errorf("entries = %+v", entries)
	}
}

func TestSwitchingToAllModeActsOnTheGamesAlreadyLookedAt(t *testing.T) {
	a, modDir, root, _, _ := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Mod 111")
	a.preserveWorkshopMods(backupTestGame, nil) // looked at, nothing at risk
	waitForBackups(t, a)
	if haveBackup(root, "111") {
		t.Fatal("at-risk mode backed up a healthy mod")
	}
	if _, err := a.SetBackupMode("all"); err != nil {
		t.Fatal(err)
	}
	waitForBackups(t, a)
	if !haveBackup(root, "111") {
		t.Error("switching to every-mod mode did not start backing up")
	}
}

func TestAModWhoseFilesSteamAlreadyRemovedIsReportedGoneNotBackedUp(t *testing.T) {
	a, modDir, root, _, _ := backupApp(t)
	content := writeWorkshopStub(t, modDir, "111", "Deleted One")
	if err := os.RemoveAll(content); err != nil { // the stub stays, the files are gone
		t.Fatal(err)
	}
	states := []WorkshopAvailability{av("111", "deleted", "page-gone")}
	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	if haveBackup(root, "111") {
		t.Error("a backup exists for a mod with no files")
	}
	a.attachBackupState(backupTestGame, states)
	if states[0].BackupState != "gone" {
		t.Errorf("BackupState = %q, want gone", states[0].BackupState)
	}
}

func TestAttachBackupStateDescribesEveryCase(t *testing.T) {
	a, modDir, _, _, _ := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Deleted One")
	states := []WorkshopAvailability{av("111", "deleted", "record"), av("999", "unlisted", "record")}
	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	a.attachBackupState(backupTestGame, states)
	if states[0].BackupState != "done" || states[0].BackedUpAt == 0 {
		t.Errorf("a backed-up mod: %+v", states[0])
	}
	if states[1].BackupState != "" {
		t.Errorf("an unlisted mod is not at risk, BackupState = %q", states[1].BackupState)
	}
	pending := []WorkshopAvailability{av("555", "private", "record")}
	a.attachBackupState(backupTestGame, pending)
	if pending[0].BackupState != "pending" {
		t.Errorf("an at-risk mod with no copy yet: %q, want pending", pending[0].BackupState)
	}
}

func TestAFailedBackupIsReportedOnceAndNotRetriedAtOnce(t *testing.T) {
	a, modDir, _, _, events := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Deleted One")
	// A backup folder that is really a file: every copy fails.
	blocked := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(blocked, []byte("x"), 0o644)
	a.backup.mu.Lock()
	a.backup.settings.Path = blocked
	a.backup.mu.Unlock()

	states := []WorkshopAvailability{av("111", "deleted", "record")}
	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	results, changed := events.snapshot()
	if len(results) != 1 || results[0].Status != "failed" || results[0].Message == "" {
		t.Fatalf("results = %+v, want one failure with a reason", results)
	}
	if changed != 0 {
		t.Error("a failure announced that backups changed")
	}

	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	if again, _ := events.snapshot(); len(again) != 1 {
		t.Errorf("the failed copy was retried at once: %d results", len(again))
	}
	a.attachBackupState(backupTestGame, states)
	if states[0].BackupState != "failed" {
		t.Errorf("BackupState = %q, want failed", states[0].BackupState)
	}
}

func TestADescriptorCannotSendACopyOutsideTheBackupFolder(t *testing.T) {
	a, modDir, root, _, _ := backupApp(t)
	// A stub whose remote_file_id tries to climb out of the backup folder.
	id := "ugc_evil"
	_ = os.MkdirAll(filepath.Join(modDir, id, "common"), 0o755)
	_ = os.WriteFile(filepath.Join(modDir, id, "common", "x.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(modDir, id+".mod"), []byte("name = \"Evil\"\npath = \""+id+"\"\nremote_file_id = \"../../escaped\"\n"), 0o644)

	a.preserveWorkshopMods(backupTestGame, []WorkshopAvailability{av("../../escaped", "deleted", "record")})
	waitForBackups(t, a)
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escaped")); err == nil {
		t.Error("a copy was written outside the backup folder")
	}
	if entries := backup.List(root, backupTestGame); len(entries) != 0 {
		t.Errorf("entries = %+v", entries)
	}
}

func TestBackupModCopiesOneWorkshopModOnRequest(t *testing.T) {
	a, modDir, root, _, events := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Mod 111")
	_ = os.MkdirAll(filepath.Join(modDir, "local_mod", "common"), 0o755)
	_ = os.WriteFile(filepath.Join(modDir, "local_mod.mod"), []byte("name = \"Local\"\npath = \"local_mod\"\n"), 0o644)

	if got, err := a.BackupMod(backupTestGame, "ugc_111"); err != nil || got != "saved" {
		t.Fatalf("BackupMod = %q, %v, want saved", got, err)
	}
	if !haveBackup(root, "111") {
		t.Fatal("no copy was made")
	}
	if e, _ := backup.Lookup(root, backupTestGame, "111"); e.Reason != backup.ReasonManual {
		t.Errorf("reason = %q, want manual", e.Reason)
	}
	if _, changed := events.snapshot(); changed != 1 {
		t.Errorf("backups-changed = %d, want 1", changed)
	}
	if got, err := a.BackupMod(backupTestGame, "ugc_111"); err != nil || got != "current" {
		t.Errorf("a second BackupMod = %q, %v, want current", got, err)
	}
	if _, err := a.BackupMod(backupTestGame, "local_mod"); err == nil || !strings.Contains(err.Error(), "Workshop") {
		t.Errorf("a local mod: err = %v", err)
	}
	if _, err := a.BackupMod(backupTestGame, "nope"); err == nil {
		t.Error("an unknown mod succeeded")
	}
	if _, err := a.BackupMod("unknown-game", "ugc_111"); err == nil {
		t.Error("an unknown game succeeded")
	}
}

func TestBackupSettingsPersistAndDefault(t *testing.T) {
	a, _, root, configDir, _ := backupApp(t)
	st := a.BackupStatus()
	if st.Mode != "atrisk" || st.Root != root || st.CustomPath != root {
		t.Errorf("status = %+v", st)
	}
	if st.DefaultRoot != filepath.Join(configDir, "Parallax Mod Backups") {
		t.Errorf("DefaultRoot = %q, want a Parallax Mod Backups folder inside the settings folder %q", st.DefaultRoot, configDir)
	}
	if st.FreeBytes <= 0 {
		t.Errorf("FreeBytes = %d, want the space left on the volume", st.FreeBytes)
	}
	if _, err := a.SetBackupMode("all"); err != nil {
		t.Fatal(err)
	}

	// A new run of the app reads the file.
	b := &App{ctx: context.Background()}
	b.initBackups(configDir)
	defer b.stopBackups()
	if got := b.BackupStatus(); got.Mode != "all" || got.Root != root {
		t.Errorf("after a restart: %+v", got)
	}

	// Going back to the default clears the custom path; a bad folder is refused and changes nothing.
	if _, err := a.SetBackupFolder(""); err != nil {
		t.Fatal(err)
	}
	if got := a.BackupStatus(); got.CustomPath != "" || got.Root != filepath.Join(configDir, "Parallax Mod Backups") {
		t.Errorf("after reset: %+v", got)
	}
	file := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if _, err := a.SetBackupFolder(file); err == nil {
		t.Error("a file was accepted as the backup folder")
	}
	if _, err := a.SetBackupFolder("relative/dir"); err == nil {
		t.Error("a relative path was accepted")
	}
}

func TestBackupOverviewCountsWorkshopModsAndBackups(t *testing.T) {
	a, modDir, _, _, _ := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Deleted One")
	writeWorkshopStub(t, modDir, "222", "Fine One")
	a.preserveWorkshopMods(backupTestGame, []WorkshopAvailability{av("111", "deleted", "record")})
	waitForBackups(t, a)

	ov, err := a.BackupOverview(backupTestGame)
	if err != nil {
		t.Fatal(err)
	}
	if ov.WorkshopMods != 2 || ov.WorkshopBytes == 0 || ov.AtRiskMods != 1 || ov.BackedUpMods != 1 || ov.BackedUpBytes == 0 {
		t.Errorf("overview = %+v", ov)
	}
	if _, err := a.BackupOverview("nope"); err == nil {
		t.Error("an unknown game succeeded")
	}
}

func TestRecheckFindsAModDeletedWhileTheAppIsOpenAndBacksItUp(t *testing.T) {
	a, modDir, root, _, events := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Mod 111")
	writeWorkshopStub(t, modDir, "222", "Mod 222")

	// Steam's answer changes between two checks: 111 is fine, then it is gone.
	deleted := false
	a.workshopDetails.SetFetch(func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
		out := map[string]steamapi.PublishedFileDetails{}
		for _, id := range ids {
			r := 1
			if id == "111" && deleted {
				r = 9
			}
			out[id] = steamapi.PublishedFileDetails{ID: id, Result: r}
		}
		return out, nil
	})
	a.workshopPages.exists = func(ctx context.Context, id string) (bool, error) { return false, nil }

	a.recheckWorkshop(context.Background(), backupTestGame)
	waitForBackups(t, a)
	if haveBackup(root, "111") || haveBackup(root, "222") {
		t.Fatal("a healthy mod was backed up")
	}

	deleted = true
	a.recheckWorkshop(context.Background(), backupTestGame)
	waitForBackups(t, a)
	if !haveBackup(root, "111") {
		t.Fatal("the mod deleted since the last check was not backed up")
	}
	if haveBackup(root, "222") {
		t.Error("a healthy mod was backed up")
	}
	countChanged := func() int {
		events.mu.Lock()
		defer events.mu.Unlock()
		n := 0
		for _, e := range events.other {
			if e == "workshop-availability-changed" {
				n++
			}
		}
		return n
	}
	if n := countChanged(); n != 1 {
		t.Errorf("workshop-availability-changed emitted %d times, want 1", n)
	}

	// Nothing changed since: nothing more is announced.
	a.recheckWorkshop(context.Background(), backupTestGame)
	waitForBackups(t, a)
	if n := countChanged(); n != 1 {
		t.Errorf("an unchanged re-check announced a change (%d events)", n)
	}
	events.mu.Lock()
	progress := 0
	for _, e := range events.other {
		if e == "backup-progress" {
			progress++
		}
	}
	events.mu.Unlock()
	if progress != 1 {
		t.Errorf("backup-progress emitted %d times, want 1 (only the copy that really happened)", progress)
	}
}

func TestWithNoFolderChosenBackupsGoInsideTheSettingsFolder(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	modDir := filepath.Join(dataHome, "Paradox Interactive", "TestGame", "mod")
	cfg := game.GameConfig{ID: backupTestGame, DisplayName: "Test Game", FolderName: "TestGame", DescriptorType: mod.DescriptorClassic, ScanFolders: []string{"common"}}
	a := &App{ctx: context.Background(), registry: game.NewRegistry([]game.GameConfig{cfg}), eventSink: (&backupEvents{}).sink}
	configDir := filepath.Join(t.TempDir(), "parallax-mod-manager")
	a.initBackups(configDir)
	t.Cleanup(a.stopBackups)
	writeWorkshopStub(t, modDir, "111", "Deleted One")

	a.preserveWorkshopMods(backupTestGame, []WorkshopAvailability{av("111", "deleted", "record")})
	waitForBackups(t, a)

	want := filepath.Join(configDir, "Parallax Mod Backups", backupTestGame, "mods", "111", "common", "x.txt")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("the copy is not in the default folder inside the settings folder: %v", err)
	}
	if st := a.BackupStatus(); st.Root != filepath.Join(configDir, "Parallax Mod Backups") || st.CustomPath != "" {
		t.Errorf("status = %+v", st)
	}
}

func TestWithNoSettingsFolderThereIsNoDefaultAndNothingIsBackedUp(t *testing.T) {
	a := &App{ctx: context.Background(), registry: game.NewRegistry(nil)}
	a.initBackups("")
	t.Cleanup(a.stopBackups)
	if st := a.BackupStatus(); st.Root != "" || st.DefaultRoot != "" {
		t.Errorf("status = %+v, want no folder", st)
	}
	if _, err := a.SetBackupFolder(""); err == nil {
		t.Error("resetting to a default that does not exist succeeded")
	}
}

// --- limits, clean-up ---------------------------------------------------------

// growMod adds a file of the given size to an installed Workshop mod.
func growMod(t *testing.T, modDir, itemID string, size int) {
	t.Helper()
	path := filepath.Join(modDir, "ugc_"+itemID, "gfx", "big.bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("z", size)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func limitEvents(e *backupEvents) []BackupLimitNotice { return e.limits() }

func (e *backupEvents) limits() []BackupLimitNotice {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]BackupLimitNotice(nil), e.limitNotices...)
}

func TestBackupsStopAtTheSizeLimitAndSayOnceThenResumeWhenRaised(t *testing.T) {
	a, modDir, root, _, events := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Deleted One")
	writeWorkshopStub(t, modDir, "222", "Deleted Two")
	growMod(t, modDir, "111", 600<<10)
	growMod(t, modDir, "222", 600<<10)
	if _, err := a.SetBackupLimits(true, 1<<20, false, 0); err != nil { // 1 MB: one of the two fits
		t.Fatal(err)
	}
	states := []WorkshopAvailability{av("111", "deleted", "record"), av("222", "deleted", "record")}

	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	if !haveBackup(root, "111") || haveBackup(root, "222") {
		t.Fatalf("111 backed up = %v, 222 backed up = %v, want the first only", haveBackup(root, "111"), haveBackup(root, "222"))
	}
	notices := limitEvents(events)
	if len(notices) != 1 || notices[0].Kind != "size" || notices[0].Waiting != 1 || !strings.Contains(notices[0].Message, "size limit") {
		t.Fatalf("limit notices = %+v, want one about the size limit with 1 mod waiting", notices)
	}
	st := a.BackupStatus()
	if st.LimitState != "size" || !st.LimitEnabled || st.UsedBytes < 600<<10 {
		t.Errorf("status = %+v", st)
	}
	held := []WorkshopAvailability{av("222", "deleted", "record")}
	a.attachBackupState(backupTestGame, held)
	if held[0].BackupState != "full" {
		t.Errorf("BackupState = %q, want full", held[0].BackupState)
	}

	// Checking again says nothing more.
	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	if n := len(limitEvents(events)); n != 1 {
		t.Errorf("the same limit was announced %d times, want once", n)
	}

	// Raising the cap resumes at once.
	if _, err := a.SetBackupLimits(true, 5<<20, false, 0); err != nil {
		t.Fatal(err)
	}
	waitForBackups(t, a)
	if !haveBackup(root, "222") {
		t.Error("raising the limit did not back up the waiting mod")
	}
	if st := a.BackupStatus(); st.LimitState != "" {
		t.Errorf("LimitState = %q after the limit was raised", st.LimitState)
	}
}

func TestDeletedAndPrivateModsAreBackedUpBeforeOthersWhenSpaceIsTight(t *testing.T) {
	a, modDir, root, _, _ := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Healthy One") // sorts before the deleted one
	writeWorkshopStub(t, modDir, "222", "Deleted Two")
	growMod(t, modDir, "111", 600<<10)
	growMod(t, modDir, "222", 600<<10)
	if _, err := a.SetBackupMode("all"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetBackupLimits(true, 1<<20, false, 0); err != nil {
		t.Fatal(err)
	}
	a.preserveWorkshopMods(backupTestGame, []WorkshopAvailability{av("222", "deleted", "record")})
	waitForBackups(t, a)
	if !haveBackup(root, "222") {
		t.Error("the deleted mod was not backed up first")
	}
	if haveBackup(root, "111") {
		t.Error("a healthy mod took the space a deleted one needed")
	}
}

func TestTheFreeSpaceGuardHoldsBackupsBackAndIsToggleable(t *testing.T) {
	a, modDir, root, _, events := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Deleted One")
	states := []WorkshopAvailability{av("111", "deleted", "record")}
	// Keep more free than any drive has.
	if _, err := a.SetBackupLimits(false, 0, true, 1<<60); err != nil {
		t.Fatal(err)
	}
	a.preserveWorkshopMods(backupTestGame, states)
	waitForBackups(t, a)
	if haveBackup(root, "111") {
		t.Fatal("a backup was made with less free space left than the user keeps free")
	}
	notices := limitEvents(events)
	if len(notices) != 1 || notices[0].Kind != "space" || !strings.Contains(notices[0].Message, "keep") {
		t.Fatalf("limit notices = %+v, want one about free space", notices)
	}
	held := append([]WorkshopAvailability(nil), states...)
	a.attachBackupState(backupTestGame, held)
	if held[0].BackupState != "lowspace" || a.BackupStatus().LimitState != "space" {
		t.Errorf("BackupState = %q, LimitState = %q", held[0].BackupState, a.BackupStatus().LimitState)
	}

	// Switching the guard off lets it through.
	if _, err := a.SetBackupLimits(false, 0, false, 1<<60); err != nil {
		t.Fatal(err)
	}
	waitForBackups(t, a)
	if !haveBackup(root, "111") {
		t.Error("with the guard off the backup was not made")
	}
}

func TestByDefaultOneGigabyteIsKeptFreeAndThereIsNoSizeCap(t *testing.T) {
	a := &App{ctx: context.Background(), registry: game.NewRegistry(nil)}
	a.initBackups(t.TempDir())
	t.Cleanup(a.stopBackups)
	st := a.BackupStatus()
	if st.LimitEnabled || !st.KeepFreeEnabled || st.KeepFreeBytes != 1<<30 || st.LimitBytes != 50<<30 {
		t.Errorf("status = %+v", st)
	}
}

func TestSetBackupLimitsValidatesAndPersists(t *testing.T) {
	a, _, _, configDir, _ := backupApp(t)
	if _, err := a.SetBackupLimits(true, 10, false, 0); err == nil {
		t.Error("a size limit of 10 bytes was accepted")
	}
	if _, err := a.SetBackupLimits(false, 0, true, -1); err == nil {
		t.Error("negative free space was accepted")
	}
	if _, err := a.SetBackupLimits(true, 3<<40, true, 2<<30); err != nil {
		t.Fatal(err)
	}
	b := &App{ctx: context.Background()}
	b.initBackups(configDir)
	defer b.stopBackups()
	st := b.BackupStatus()
	if !st.LimitEnabled || st.LimitBytes != 3<<40 || !st.KeepFreeEnabled || st.KeepFreeBytes != 2<<30 {
		t.Errorf("after a restart: %+v", st)
	}
}

// cleanupApp has four backups: 111 installed and available, 222 installed but deleted
// on the Workshop, 333 no longer installed (backed up under "all"), 444 no longer
// installed (backed up because it was deleted).
func cleanupApp(t *testing.T) (a *App, modDir, root string, events *backupEvents) {
	t.Helper()
	a, modDir, root, _, events = backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Available One")
	writeWorkshopStub(t, modDir, "222", "Deleted Two")
	a.workshopDetails.SetFetch(func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
		out := map[string]steamapi.PublishedFileDetails{}
		for _, id := range ids {
			r := 1
			if id == "222" {
				r = 9
			}
			out[id] = steamapi.PublishedFileDetails{ID: id, Result: r}
		}
		return out, nil
	})
	a.workshopPages.exists = func(ctx context.Context, id string) (bool, error) { return false, nil }

	old := t.TempDir()
	for id, reason := range map[string]string{"111": backup.ReasonAll, "222": backup.ReasonDeleted, "333": backup.ReasonAll, "444": backup.ReasonDeleted} {
		src := filepath.Join(old, id)
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "descriptor.mod"), []byte("name=\"m"+id+"\"\n"+strings.Repeat("x", 1000)), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := backup.Copy(context.Background(), root, backup.Source{GameID: backupTestGame, ModID: "ugc_" + id, RemoteFileID: id, Name: "Mod " + id, ContentPath: src, Reason: reason}, nil); err != nil {
			t.Fatal(err)
		}
	}
	return a, modDir, root, events
}

func ids(entries []backup.Entry) string {
	var out []string
	for _, e := range entries {
		out = append(out, e.RemoteFileID)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestCleanupPlansAreCautious(t *testing.T) {
	a, _, _, _ := cleanupApp(t)
	installed, err := a.PlanBackupCleanup(backupTestGame, "installed")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(installed.Entries); got != "111" || installed.Bytes == 0 {
		t.Errorf("installed plan = %q (%d bytes), want only the installed, still available 111", got, installed.Bytes)
	}
	unavailable, err := a.PlanBackupCleanup(backupTestGame, "unavailable")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(unavailable.Entries); got != "222,444" {
		t.Errorf("unavailable plan = %q, want the deleted 222 and 444 (copied because deleted)", got)
	}
	if _, err := a.PlanBackupCleanup(backupTestGame, "everything"); err == nil {
		t.Error("an unknown clean-up was accepted")
	}
	if _, err := a.PlanBackupCleanup("unknown-game", "installed"); err == nil {
		t.Error("an unknown game was accepted")
	}
}

func TestCleanupOfInstalledNeedsSteamButUnavailableFallsBackToWhyItWasCopied(t *testing.T) {
	a, _, _, _ := cleanupApp(t)
	a.workshopDetails.SetFetch(func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
		return nil, errors.New("steam is unreachable")
	})
	a.workshopDetails.Forget()
	if _, err := a.PlanBackupCleanup(backupTestGame, "installed"); err == nil {
		t.Error("with Steam unreachable the app selected backups as redundant without knowing")
	}
	plan, err := a.PlanBackupCleanup(backupTestGame, "unavailable")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(plan.Entries); got != "222,444" {
		t.Errorf("unavailable plan without Steam = %q, want the ones copied because they were deleted", got)
	}
}

func TestDeleteBackupsRemovesOnlyWhatIsStillACandidate(t *testing.T) {
	a, modDir, root, events := cleanupApp(t)
	// The redundant ones: 222 is asked for too but is not redundant (it is deleted on the Workshop).
	res, err := a.DeleteBackups(backupTestGame, "installed", []string{"111", "222", "999", "../../x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 || res.Skipped != 3 || res.Bytes == 0 {
		t.Errorf("result = %+v, want 1 deleted and 3 left alone", res)
	}
	if haveBackup(root, "111") || !haveBackup(root, "222") {
		t.Errorf("111 still backed up = %v, 222 still backed up = %v", haveBackup(root, "111"), haveBackup(root, "222"))
	}
	if _, err := os.Stat(filepath.Join(modDir, "ugc_111", "common", "x.txt")); err != nil {
		t.Error("deleting a backup touched the mod itself")
	}
	if _, changed := events.snapshot(); changed == 0 {
		t.Error("deleting announced no change")
	}

	// The not recommended one, and by hand.
	res, err = a.DeleteBackups(backupTestGame, "unavailable", []string{"222", "444"})
	if err != nil || res.Deleted != 2 {
		t.Errorf("unavailable: %+v, %v", res, err)
	}
	res, err = a.DeleteBackups(backupTestGame, "selected", []string{"333"})
	if err != nil || res.Deleted != 1 {
		t.Errorf("selected: %+v, %v", res, err)
	}
	if left := backup.List(root, backupTestGame); len(left) != 0 {
		t.Errorf("left = %+v", left)
	}
	if _, err := a.DeleteBackups(backupTestGame, "nonsense", []string{"1"}); err == nil {
		t.Error("an unknown clean-up deleted something")
	}
}

func TestMakingRoomStartsTheBackupsThatWereWaiting(t *testing.T) {
	a, modDir, root, _ := cleanupApp(t)
	// The deleted mod 222 is installed and waits: the cap is already used by the old backups.
	growMod(t, modDir, "222", 900<<10)
	if err := os.RemoveAll(backup.FolderFor(root, backupTestGame, "222")); err != nil {
		t.Fatal(err)
	}
	// Old backups: three small ones (about 3 KB). Set the cap just above them.
	used := backup.TotalSize(root)
	if _, err := a.SetBackupLimits(true, max(used+(10<<10), backup.MinLimitBytes), false, 0); err != nil {
		t.Fatal(err)
	}
	// Fill the cap with a big backup of a mod that is no longer installed.
	big := filepath.Join(t.TempDir(), "555")
	_ = os.MkdirAll(big, 0o755)
	_ = os.WriteFile(filepath.Join(big, "big.bin"), []byte(strings.Repeat("b", 950<<10)), 0o644)
	if _, err := backup.Copy(context.Background(), root, backup.Source{GameID: backupTestGame, ModID: "ugc_555", RemoteFileID: "555", Name: "Big", ContentPath: big, Reason: backup.ReasonAll}, nil); err != nil {
		t.Fatal(err)
	}

	a.preserveWorkshopMods(backupTestGame, []WorkshopAvailability{av("222", "deleted", "page-gone")})
	waitForBackups(t, a)
	if haveBackup(root, "222") {
		t.Fatal("the backup was made although the cap is used up")
	}
	if a.BackupStatus().LimitState != "size" {
		t.Fatal("the limit state was not set")
	}

	// Deleting the big backup makes room, and the waiting mod is backed up without being asked.
	if _, err := a.DeleteBackups(backupTestGame, "selected", []string{"555"}); err != nil {
		t.Fatal(err)
	}
	waitForBackups(t, a)
	if !haveBackup(root, "222") {
		t.Error("making room did not start the backup that was waiting")
	}
	if a.BackupStatus().LimitState != "" {
		t.Errorf("LimitState = %q, want it cleared", a.BackupStatus().LimitState)
	}
}

func TestBackUpNowSaysWhyWhenALimitStopsIt(t *testing.T) {
	a, modDir, _, _, _ := backupApp(t)
	writeWorkshopStub(t, modDir, "111", "Mod 111")
	growMod(t, modDir, "111", 900<<10)
	if _, err := a.SetBackupLimits(true, backup.MinLimitBytes, false, 0); err != nil {
		t.Fatal(err)
	}
	// Nothing is used yet, so it fits.
	if got, err := a.BackupMod(backupTestGame, "ugc_111"); err != nil || got != "saved" {
		t.Fatalf("BackupMod = %q, %v", got, err)
	}
	writeWorkshopStub(t, modDir, "222", "Mod 222")
	growMod(t, modDir, "222", 900<<10)
	_, err := a.BackupMod(backupTestGame, "ugc_222")
	if err == nil || !strings.Contains(err.Error(), "size limit") || !strings.Contains(err.Error(), "Settings > Backup") {
		t.Errorf("err = %v, want a message about the size limit pointing at Settings > Backup", err)
	}
	if _, err := a.SetBackupLimits(false, 0, true, 1<<60); err != nil {
		t.Fatal(err)
	}
	if _, err := a.BackupMod(backupTestGame, "ugc_222"); err == nil || !strings.Contains(err.Error(), "free space") {
		t.Errorf("err = %v, want a message about free space", err)
	}
}
