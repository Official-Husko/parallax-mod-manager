package app

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabtracking"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// buildTestZip returns a real zip archive's bytes, built from entries (name -> content).
func buildTestZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// zipServer serves body (a real zip archive's bytes) for every request - a stand-in
// for LoversLab's own download URL, which loverslab.Client.DownloadFile happily
// fetches from regardless of host (see internal/loverslab/client.go's newRequest -
// there is nothing loverslab.com-specific about it), so this exercises the real HTTP
// download code path without any real network dependency.
func zipServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type installEnv struct {
	a       *App
	cfg     game.GameConfig
	modDir  string
	events  []string
	eventMu sync.Mutex
}

func (e *installEnv) sawEvent(name string) bool {
	e.eventMu.Lock()
	defer e.eventMu.Unlock()
	for _, n := range e.events {
		if n == name {
			return true
		}
	}
	return false
}

// newInstallEnv is an App whose game exists (an empty mod folder, no fixture mods -
// LoversLabInstall's own tests only care about what installing itself writes) and
// whose LoversLab session is already "signed in": a client set directly, verified
// recently enough that ensureLoversLabSession returns it without touching mgr/login/
// verify or any network at all (see loverslab.go's own recheck-interval caching).
func newInstallEnv(t *testing.T) *installEnv {
	t.Helper()
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	env := &installEnv{cfg: game.Stellaris}
	env.modDir = filepath.Join(data, "Paradox Interactive", "Stellaris", "mod")
	if err := os.MkdirAll(env.modDir, 0o755); err != nil {
		t.Fatal(err)
	}

	client, err := loverslab.New()
	if err != nil {
		t.Fatal(err)
	}

	env.a = &App{
		ctx:          context.Background(),
		registry:     game.NewRegistry([]game.GameConfig{env.cfg}),
		preferences:  preferences.Defaults(),
		configAppDir: t.TempDir(),
		eventSink: func(name string, args ...any) {
			env.eventMu.Lock()
			defer env.eventMu.Unlock()
			env.events = append(env.events, name)
		},
	}
	env.a.loverslab.client = client
	env.a.loverslab.verifiedAt = time.Now()
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{}, errors.New("getFileDetail: not scripted for this test")
	}
	env.a.loverslabInstalls = loverslabtracking.Store{Dir: filepath.Join(env.a.configAppDir, "loverslab_installs")}
	return env
}

func TestLoversLabInstallFreshInstallWithTheArchivesOwnDescriptor(t *testing.T) {
	env := newInstallEnv(t)
	srv := zipServer(t, buildTestZip(t, map[string]string{
		"descriptor.mod": "name=\"Real Mod\"\nversion=\"1.2\"\n",
		"common/x.txt":   "x = 1",
	}))

	file := loverslab.FileSummary{ID: 31347, Title: "Stable Portraits", URL: "https://www.loverslab.com/files/file/31347-stable-portraits/", Updated: "2 days ago"}
	res, err := env.a.LoversLabInstall(env.cfg.ID, "req-1", file, "2026-09-13T20:01:52+0200", []loverslab.FileDownload{{URL: srv.URL}})
	if err != nil {
		t.Fatalf("LoversLabInstall: %v", err)
	}
	// The archive shipped its own descriptor.mod, so only the stub is written -
	// nothing synthesized on top of it.
	if len(res.Files) != 1 {
		t.Fatalf("wrote %v, want just the stub", res.Files)
	}

	contentDir := filepath.Join(env.modDir, "Stable Portraits")
	if data, err := os.ReadFile(filepath.Join(contentDir, "descriptor.mod")); err != nil || string(data) != "name=\"Real Mod\"\nversion=\"1.2\"\n" {
		t.Errorf("the archive's own descriptor.mod was not kept as-is: %q, %v", data, err)
	}
	if _, err := os.ReadFile(filepath.Join(contentDir, "common/x.txt")); err != nil {
		t.Errorf("common/x.txt was not extracted: %v", err)
	}

	stubPath := filepath.Join(env.modDir, "loverslab_31347.mod")
	stub, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("the stub was not written: %v", err)
	}
	if !bytes.Contains(stub, []byte(`remote_file_id="31347"`)) {
		t.Errorf("stub does not carry the LoversLab file id: %s", stub)
	}
	if !bytes.Contains(stub, []byte(`path="`+contentDir+`"`)) {
		t.Errorf("stub does not point at the content folder: %s", stub)
	}

	if !env.sawEvent("mods-changed") {
		t.Error("mods-changed was not emitted")
	}
}

func TestLoversLabInstallSynthesizesADescriptorWhenTheArchiveHasNone(t *testing.T) {
	env := newInstallEnv(t)
	srv := zipServer(t, buildTestZip(t, map[string]string{"common/x.txt": "x = 1"}))

	file := loverslab.FileSummary{ID: 999, Title: "No Descriptor Mod", URL: "https://www.loverslab.com/files/file/999-no-descriptor/", Updated: "today"}
	res, err := env.a.LoversLabInstall(env.cfg.ID, "req-2", file, "2026-09-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv.URL}})
	if err != nil {
		t.Fatalf("LoversLabInstall: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("wrote %v, want a synthesized descriptor.mod and the stub", res.Files)
	}

	contentDir := filepath.Join(env.modDir, "No Descriptor Mod")
	desc, err := os.ReadFile(filepath.Join(contentDir, "descriptor.mod"))
	if err != nil {
		t.Fatalf("no descriptor.mod was synthesized: %v", err)
	}
	if !bytes.Contains(desc, []byte(`name="No Descriptor Mod"`)) {
		t.Errorf("synthesized descriptor.mod = %s, want the file's own title as the name", desc)
	}
}

func TestLoversLabInstallUpdatesInPlaceRatherThanDuplicating(t *testing.T) {
	env := newInstallEnv(t)
	file := loverslab.FileSummary{ID: 500, Title: "Evolving Mod", URL: "https://www.loverslab.com/files/file/500-evolving-mod/", Updated: "version 1"}

	srv1 := zipServer(t, buildTestZip(t, map[string]string{"descriptor.mod": "name=\"Evolving Mod\"\nversion=\"1.0\"\n", "common/old.txt": "old"}))
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-3a", file, "2026-01-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv1.URL}}); err != nil {
		t.Fatalf("first LoversLabInstall: %v", err)
	}

	file.Updated = "version 2"
	srv2 := zipServer(t, buildTestZip(t, map[string]string{"descriptor.mod": "name=\"Evolving Mod\"\nversion=\"2.0\"\n", "common/new.txt": "new"}))
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-3b", file, "2026-02-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv2.URL}}); err != nil {
		t.Fatalf("second LoversLabInstall: %v", err)
	}

	contentDir := filepath.Join(env.modDir, "Evolving Mod")
	if _, err := os.Stat(filepath.Join(contentDir, "common/new.txt")); err != nil {
		t.Errorf("the updated content is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(contentDir, "common/old.txt")); !os.IsNotExist(err) {
		t.Errorf("the previous version's file should have been removed on update: %v", err)
	}

	entries, err := os.ReadDir(env.modDir)
	if err != nil {
		t.Fatal(err)
	}
	var folders int
	for _, e := range entries {
		if e.IsDir() {
			folders++
		}
	}
	if folders != 1 {
		t.Errorf("found %d content folders in the mod dir, want exactly 1 (no duplicate)", folders)
	}
}

func TestLoversLabInstallTracksTheInstallForUpdateChecking(t *testing.T) {
	env := newInstallEnv(t)
	srv := zipServer(t, buildTestZip(t, map[string]string{"descriptor.mod": "name=\"Tracked\"\n"}))
	file := loverslab.FileSummary{
		ID: 42, Title: "Tracked Mod", URL: "https://www.loverslab.com/files/file/42-tracked-mod/", Updated: "yesterday",
		ThumbnailURL: "https://static.loverslab.com/files/42/thumb.jpg",
	}

	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-4", file, "2026-03-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv.URL}}); err != nil {
		t.Fatalf("LoversLabInstall: %v", err)
	}

	installs, err := env.a.loverslabInstalls.Load(env.cfg.ID)
	if err != nil {
		t.Fatalf("loading tracked installs: %v", err)
	}
	entry, ok := installs["loverslab_42"]
	if !ok {
		t.Fatalf("no tracked entry for loverslab_42: %+v", installs)
	}
	if entry.FileURL != file.URL || entry.FileID != 42 || entry.Title != "Tracked Mod" || entry.InstalledDateModified != "2026-03-01T00:00:00+0000" || entry.ThumbnailURL != file.ThumbnailURL {
		t.Errorf("tracked entry = %+v", entry)
	}
	if entry.InstalledAt == 0 {
		t.Error("InstalledAt was not set")
	}
}

// TestLoversLabInstallCombinesMultipleSelectedFilesIntoOneModFolder is the real
// batch-install case the Files tab's checkboxes drive: picking a main archive
// together with a separate addon zip must land both inside the same content
// folder, not overwrite one with the other.
func TestLoversLabInstallCombinesMultipleSelectedFilesIntoOneModFolder(t *testing.T) {
	env := newInstallEnv(t)
	main := zipServer(t, buildTestZip(t, map[string]string{
		"descriptor.mod":  "name=\"Lustful Void\"\nversion=\"0.8.0\"\n",
		"common/main.txt": "main content",
	}))
	// Two top-level folders, like a real Paradox addon zip (common/, events/, gfx/,
	// ...) - a single bare top-level folder would be treated as a wrapper to strip
	// (see ExtractZip's own contentLayout), which a real addon archive practically
	// never is.
	addon := zipServer(t, buildTestZip(t, map[string]string{
		"common/addon.txt":        "addon content",
		"events/addon_events.txt": "addon events",
	}))

	file := loverslab.FileSummary{ID: 8719, Title: "Lustful Void", URL: "https://www.loverslab.com/files/file/8719-lustful-void/", Updated: "today"}
	res, err := env.a.LoversLabInstall(env.cfg.ID, "req-batch", file, "2026-09-13T20:01:52+0200", []loverslab.FileDownload{
		{Name: "Lustful Void 0.8.0.zip", URL: main.URL},
		{Name: "LV Lewd Rooms.zip", URL: addon.URL},
	})
	if err != nil {
		t.Fatalf("LoversLabInstall: %v", err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("wrote %v, want just the stub (the main archive shipped its own descriptor.mod)", res.Files)
	}

	contentDir := filepath.Join(env.modDir, "Lustful Void")
	if _, err := os.Stat(filepath.Join(contentDir, "common/main.txt")); err != nil {
		t.Errorf("the main archive's own content is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(contentDir, "common/addon.txt")); err != nil {
		t.Errorf("the addon zip's own content is missing - a batch install must not overwrite earlier selections: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(contentDir, "descriptor.mod")); err != nil || string(data) != "name=\"Lustful Void\"\nversion=\"0.8.0\"\n" {
		t.Errorf("the main archive's own descriptor.mod was not kept: %q, %v", data, err)
	}
}

// TestLoversLabInstallBatchUpdateReplacesEverythingFromTheOldSelection is the
// update-in-place equivalent: a previously installed multi-file batch, updated with
// a smaller new selection, must not leave anything from the old one behind (the
// existing single-file update test already covers wiping content the new archive
// no longer ships; this covers wiping content an earlier addon in the old batch
// shipped that isn't part of the new one either).
func TestLoversLabInstallBatchUpdateReplacesEverythingFromTheOldSelection(t *testing.T) {
	env := newInstallEnv(t)
	file := loverslab.FileSummary{ID: 501, Title: "Evolving Bundle", URL: "https://www.loverslab.com/files/file/501-evolving-bundle/", Updated: "v1"}

	main1 := zipServer(t, buildTestZip(t, map[string]string{"descriptor.mod": "name=\"Evolving Bundle\"\n", "common/main.txt": "v1"}))
	addon1 := zipServer(t, buildTestZip(t, map[string]string{"common/old-addon.txt": "old addon"}))
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-b1", file, "2026-01-01T00:00:00+0000", []loverslab.FileDownload{
		{Name: "main.zip", URL: main1.URL},
		{Name: "old-addon.zip", URL: addon1.URL},
	}); err != nil {
		t.Fatalf("first LoversLabInstall: %v", err)
	}

	main2 := zipServer(t, buildTestZip(t, map[string]string{"descriptor.mod": "name=\"Evolving Bundle\"\n", "common/main.txt": "v2"}))
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-b2", file, "2026-02-01T00:00:00+0000", []loverslab.FileDownload{
		{Name: "main.zip", URL: main2.URL},
	}); err != nil {
		t.Fatalf("second LoversLabInstall: %v", err)
	}

	contentDir := filepath.Join(env.modDir, "Evolving Bundle")
	if data, err := os.ReadFile(filepath.Join(contentDir, "common/main.txt")); err != nil || string(data) != "v2" {
		t.Errorf("common/main.txt = %q, %v, want the new selection's content", data, err)
	}
	if _, err := os.Stat(filepath.Join(contentDir, "common/old-addon.txt")); !os.IsNotExist(err) {
		t.Errorf("the old batch's addon content should have been wiped, not left behind: %v", err)
	}
}

func TestLoversLabInstallWithNoFilesSelectedIsAClearError(t *testing.T) {
	env := newInstallEnv(t)
	file := loverslab.FileSummary{ID: 1, Title: "Nothing Selected", URL: "https://www.loverslab.com/files/file/1-nothing-selected/"}
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-empty", file, "", nil); err == nil {
		t.Error("expected an error when no files are selected")
	}
}

func TestLoversLabInstallRejectsANonZipDownloadAndWritesNothing(t *testing.T) {
	env := newInstallEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not a zip file at all"))
	}))
	defer srv.Close()

	file := loverslab.FileSummary{ID: 7, Title: "Bad Archive", URL: "https://www.loverslab.com/files/file/7-bad-archive/", Updated: "today"}
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-5", file, "2026-03-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv.URL}}); err == nil {
		t.Error("expected an error for a non-zip download")
	}
	if _, err := os.Stat(filepath.Join(env.modDir, "Bad Archive")); !os.IsNotExist(err) {
		t.Errorf("a partial folder was left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.modDir, "loverslab_7.mod")); !os.IsNotExist(err) {
		t.Errorf("a stub was left behind despite the failure: %v", err)
	}
}

func TestCancellingALoversLabInstallLeavesNoPartialFolderBehind(t *testing.T) {
	env := newInstallEnv(t)
	entries := map[string]string{"descriptor.mod": "name=\"Cancel Me\"\n"}
	for i := 0; i < 200; i++ {
		entries[fmt.Sprintf("common/pad%d.txt", i)] = "padding so the download body is large enough to arrive in more than one chunk of bytes, which is what makes more than one progress event fire before the whole thing finishes downloading"
	}
	srv := zipServer(t, buildTestZip(t, entries))

	const requestID = "cancel-me"
	seen := 0
	env.a.eventSink = func(name string, args ...any) {
		env.eventMu.Lock()
		env.events = append(env.events, name)
		env.eventMu.Unlock()
		if name == "loverslab-install-progress" {
			seen++
			if seen == 2 {
				env.a.CancelLoversLabInstall(requestID)
			}
		}
	}

	file := loverslab.FileSummary{ID: 8, Title: "Cancel Me", URL: "https://www.loverslab.com/files/file/8-cancel-me/", Updated: "today"}
	_, err := env.a.LoversLabInstall(env.cfg.ID, requestID, file, "2026-03-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv.URL}})
	if err == nil {
		t.Fatal("LoversLabInstall: want an error after cancelling, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(env.modDir, "Cancel Me")); !os.IsNotExist(statErr) {
		t.Errorf("a partial folder was left behind: %v", statErr)
	}
}

func TestCancelLoversLabInstallOnAnUnknownRequestIsANoOp(t *testing.T) {
	env := newInstallEnv(t)
	env.a.CancelLoversLabInstall("no-such-request") // must not panic
}

func TestUninstallLoversLabModRemovesContentStubAndTrackingEntry(t *testing.T) {
	env := newInstallEnv(t)
	srv := zipServer(t, buildTestZip(t, map[string]string{
		"descriptor.mod": "name=\"Removable\"\nversion=\"1.0\"\n",
	}))
	file := loverslab.FileSummary{ID: 900, Title: "Removable", URL: "https://www.loverslab.com/files/file/900-removable/", Updated: "today"}
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-u1", file, "2026-01-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv.URL}}); err != nil {
		t.Fatalf("LoversLabInstall: %v", err)
	}
	contentDir := filepath.Join(env.modDir, "Removable")
	stubPath := filepath.Join(env.modDir, "loverslab_900.mod")
	if _, err := os.Stat(contentDir); err != nil {
		t.Fatalf("setup: content dir missing: %v", err)
	}

	if err := env.a.UninstallLoversLabMod(env.cfg.ID, 900); err != nil {
		t.Fatalf("UninstallLoversLabMod: %v", err)
	}
	if _, err := os.Stat(contentDir); !os.IsNotExist(err) {
		t.Errorf("content dir was not removed: %v", err)
	}
	if _, err := os.Stat(stubPath); !os.IsNotExist(err) {
		t.Errorf("stub was not removed: %v", err)
	}
	installs, err := env.a.loverslabInstalls.Load(env.cfg.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := installs["loverslab_900"]; ok {
		t.Error("the tracking entry was not removed")
	}
	if !env.sawEvent("mods-changed") {
		t.Error("mods-changed was not emitted")
	}
}

func TestUninstallLoversLabModOnAModThatIsNotInstalledReturnsAClearError(t *testing.T) {
	env := newInstallEnv(t)
	err := env.a.UninstallLoversLabMod(env.cfg.ID, 12345)
	if err == nil {
		t.Fatal("expected an error for a mod that was never installed")
	}
}

func TestLoversLabInstalledModsReflectsTheTrackingStoreSortedNewestFirst(t *testing.T) {
	env := newInstallEnv(t)
	installs, err := env.a.loverslabInstalls.Load(env.cfg.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	installs, _ = loverslabtracking.With(installs, "loverslab_1", loverslabtracking.Entry{
		FileURL: "https://www.loverslab.com/files/file/1-older/", FileID: 1, Title: "Older", InstalledAt: 100,
	})
	installs, _ = loverslabtracking.With(installs, "loverslab_2", loverslabtracking.Entry{
		FileURL: "https://www.loverslab.com/files/file/2-newer/", FileID: 2, Title: "Newer", InstalledAt: 200,
		ThumbnailURL: "https://static.loverslab.com/files/2/thumb.jpg",
	})
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := env.a.LoversLabInstalledMods(env.cfg.ID)
	if err != nil {
		t.Fatalf("LoversLabInstalledMods: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Title != "Newer" || got[1].Title != "Older" {
		t.Errorf("not sorted newest-first: %+v", got)
	}
	if got[0].ThumbnailURL != "https://static.loverslab.com/files/2/thumb.jpg" {
		t.Errorf("ThumbnailURL = %q, want the tracked entry's own", got[0].ThumbnailURL)
	}
	if got[1].ThumbnailURL != "" {
		t.Errorf("ThumbnailURL = %q, want empty for an entry tracked before this field existed", got[1].ThumbnailURL)
	}
	// Neither was actually extracted to disk in this test - both should read as
	// missing rather than crash or silently claim they're present.
	if !got[0].ContentMissing || !got[1].ContentMissing {
		t.Errorf("expected both entries to be flagged ContentMissing: %+v", got)
	}
}

func TestLoversLabInstalledModsFlagsContentPresentAfterARealInstall(t *testing.T) {
	env := newInstallEnv(t)
	srv := zipServer(t, buildTestZip(t, map[string]string{
		"descriptor.mod": "name=\"Present\"\nversion=\"1.0\"\n",
	}))
	file := loverslab.FileSummary{ID: 901, Title: "Present", URL: "https://www.loverslab.com/files/file/901-present/", Updated: "today"}
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-u2", file, "2026-01-01T00:00:00+0000", []loverslab.FileDownload{{URL: srv.URL}}); err != nil {
		t.Fatalf("LoversLabInstall: %v", err)
	}

	got, err := env.a.LoversLabInstalledMods(env.cfg.ID)
	if err != nil {
		t.Fatalf("LoversLabInstalledMods: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].ContentMissing {
		t.Error("a freshly installed mod should not be flagged ContentMissing")
	}
	if got[0].FileID != 901 || got[0].Title != "Present" {
		t.Errorf("unexpected entry: %+v", got[0])
	}
}
