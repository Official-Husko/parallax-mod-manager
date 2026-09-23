package main

import (
	"archive/zip"
	"bytes"
	"context"
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
	res, err := env.a.LoversLabInstall(env.cfg.ID, "req-1", file, srv.URL)
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
	res, err := env.a.LoversLabInstall(env.cfg.ID, "req-2", file, srv.URL)
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
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-3a", file, srv1.URL); err != nil {
		t.Fatalf("first LoversLabInstall: %v", err)
	}

	file.Updated = "version 2"
	srv2 := zipServer(t, buildTestZip(t, map[string]string{"descriptor.mod": "name=\"Evolving Mod\"\nversion=\"2.0\"\n", "common/new.txt": "new"}))
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-3b", file, srv2.URL); err != nil {
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
	file := loverslab.FileSummary{ID: 42, Title: "Tracked Mod", URL: "https://www.loverslab.com/files/file/42-tracked-mod/", Updated: "yesterday"}

	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-4", file, srv.URL); err != nil {
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
	if entry.FileURL != file.URL || entry.FileID != 42 || entry.Title != "Tracked Mod" || entry.InstalledUpdated != "yesterday" {
		t.Errorf("tracked entry = %+v", entry)
	}
	if entry.InstalledAt == 0 {
		t.Error("InstalledAt was not set")
	}
}

func TestLoversLabInstallRejectsANonZipDownloadAndWritesNothing(t *testing.T) {
	env := newInstallEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not a zip file at all"))
	}))
	defer srv.Close()

	file := loverslab.FileSummary{ID: 7, Title: "Bad Archive", URL: "https://www.loverslab.com/files/file/7-bad-archive/", Updated: "today"}
	if _, err := env.a.LoversLabInstall(env.cfg.ID, "req-5", file, srv.URL); err == nil {
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
	_, err := env.a.LoversLabInstall(env.cfg.ID, requestID, file, srv.URL)
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
