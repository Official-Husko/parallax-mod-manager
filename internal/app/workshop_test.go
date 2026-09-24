package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
	"github.com/Official-Husko/parallax-mod-manager/internal/workshop"
)

// fakePublisher is a workshop.Publisher that never touches a real helper
// process - it records what it was asked to do and returns whatever the
// test pre-loaded.
type fakePublisher struct {
	gotReq        workshop.PublishRequest
	result        workshop.PublishResult
	err           error
	progressCalls []workshop.PublishProgress
}

func (f *fakePublisher) Publish(_ context.Context, req workshop.PublishRequest, onProgress func(workshop.PublishProgress)) (workshop.PublishResult, error) {
	f.gotReq = req
	if onProgress != nil {
		p := workshop.PublishProgress{Stage: "opening"}
		onProgress(p)
		f.progressCalls = append(f.progressCalls, p)
	}
	return f.result, f.err
}

const testModName = "My Mod"

// newWorkshopTestApp builds an App wired for Stellaris only, with its
// install path overridden to installDir (when non-empty) and one real,
// scannable local mod named testModName dropped into an "extra" mod folder
// (see preferences.ExtraModFolders) - light enough not to need the full
// launcher-mod-dir/XDG_DATA_HOME machinery editorEnv (modedit_linux_test.go)
// sets up, since this only ever needs library.ModFolderPath to resolve one
// mod's own real content folder, never a full launcher-integration scan.
//
// Returns the mod's own real ID as scan.ScanExtraFolder actually assigns it
// (an xhash of its descriptor path, per that function's own doc comment) -
// never assumed to be testModName itself, which is a different thing (the
// mod's display Name).
func newWorkshopTestApp(t *testing.T, installDir string) (*App, string) {
	t.Helper()
	// Without this, game.GameConfig.UserDataDir() (and so library.LoadGame's
	// scan of the launcher's own mod folder) resolves to this machine's real
	// ~/.local/share/Paradox Interactive/Stellaris - a real person's real,
	// possibly large and unrelated mod list, not a test fixture. Same
	// isolation editorEnv (modedit_linux_test.go) already uses.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	extra := t.TempDir()
	modPath := filepath.Join(extra, testModName)
	if err := os.MkdirAll(filepath.Join(modPath, "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	descriptor := "name=\"" + testModName + "\"\nversion=\"1.0\"\nsupported_version=\"v4.*\"\n"
	if err := os.WriteFile(filepath.Join(modPath, "descriptor.mod"), []byte(descriptor), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modPath, "common", "a.txt"), []byte("x = 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	prefs := preferences.Defaults()
	prefs.ExtraModFolders = map[string][]string{game.Stellaris.ID: {extra}}
	a := &App{
		ctx:         context.Background(),
		registry:    game.NewRegistry([]game.GameConfig{game.Stellaris}),
		preferences: prefs,
		eventSink:   func(string, ...any) {}, // never call the real Wails runtime in tests
	}
	if installDir != "" {
		a.preferences.GamePaths = map[string]string{game.Stellaris.ID: installDir}
	}

	// library.LoadGame directly, not a.ScanGame: that App-level wrapper emits
	// "scan-quick" via a raw, ungated wailsruntime.EventsEmit(a.ctx, ...) call
	// (app.go) that isn't routed through a.emit/eventSink at all, so it fatally
	// rejects a plain context.Background() like this test app's - no existing
	// test in this package calls a.ScanGame directly, for exactly that reason.
	summary, err := library.LoadGame(a.ctx, game.Stellaris, library.Options{ExtraFolders: a.extraModFolders(game.Stellaris.ID)})
	if err != nil {
		t.Fatalf("scanning the fixture mod: %v", err)
	}
	if len(summary.Mods) != 1 {
		t.Fatalf("fixture produced %d mods, want exactly 1: %+v", len(summary.Mods), summary.Mods)
	}
	return a, summary.Mods[0].ID
}

// realLibraryName is this host's own real Steamworks library filename -
// steamLibraryPath only ever looks for the one matching runtime.GOOS.
func realLibraryName() string {
	switch runtime.GOOS {
	case "windows":
		return "steam_api64.dll"
	case "darwin":
		return "libsteam_api.dylib"
	default:
		return "libsteam_api.so"
	}
}

// newStellarisInstallDir builds a directory that VerifyInstallDir accepts
// for game.Stellaris (its own SignatureFiles) and optionally drops in a
// fake Steamworks library matching this host's OS.
func newStellarisInstallDir(t *testing.T, withLibrary bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "launcher-settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if withLibrary {
		if err := os.WriteFile(filepath.Join(dir, realLibraryName()), []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSteamLibraryPathFindsTheRealLibraryForThisHostOS(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	got, err := steamLibraryPath(dir)
	if err != nil {
		t.Fatalf("steamLibraryPath() error = %v", err)
	}
	want := filepath.Join(dir, realLibraryName())
	if got != want {
		t.Errorf("steamLibraryPath() = %q, want %q", got, want)
	}
}

func TestSteamLibraryPathFailsCleanlyWhenTheLibraryIsMissing(t *testing.T) {
	dir := newStellarisInstallDir(t, false)
	_, err := steamLibraryPath(dir)
	if err == nil {
		t.Fatal("want an error when no Steamworks library is present")
	}
}

func TestPublishModToWorkshopRejectsAnUnknownGame(t *testing.T) {
	a, modID := newWorkshopTestApp(t, "")
	_, err := a.PublishModToWorkshop("no-such-game", modID, WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error for an unknown game id")
	}
}

func TestPublishModToWorkshopFailsWhenInstallCannotBeFound(t *testing.T) {
	a, modID := newWorkshopTestApp(t, "") // no override, and DetectInstall will find nothing real on the test machine
	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error when the game's install cannot be resolved")
	}
}

func TestPublishModToWorkshopFailsWhenTheInstallHasNoSteamworksLibrary(t *testing.T) {
	dir := newStellarisInstallDir(t, false) // valid install, but no library file
	a, modID := newWorkshopTestApp(t, dir)
	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error when the install has no Steamworks library")
	}
}

func TestPublishModToWorkshopFailsWhenTheModCannotBeFound(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, _ := newWorkshopTestApp(t, dir)
	a.workshopPublisher = &fakePublisher{}

	_, err := a.PublishModToWorkshop(game.Stellaris.ID, "no-such-mod", WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error when the mod itself cannot be found")
	}
}

func TestPublishModToWorkshopPassesTheResolvedAppIDLibraryAndContentPathToThePublisher(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	fake := &fakePublisher{result: workshop.PublishResult{PublishedFileID: 12345}}
	a.workshopPublisher = fake

	result, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, WorkshopPublishRequest{
		Title:      testModName,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("PublishModToWorkshop() error = %v", err)
	}
	if result.PublishedFileID != "12345" {
		t.Errorf("PublishedFileID = %q, want \"12345\"", result.PublishedFileID)
	}
	if fake.gotReq.AppID != 281990 {
		t.Errorf("Publisher got AppID %d, want 281990", fake.gotReq.AppID)
	}
	if fake.gotReq.LibraryPath != filepath.Join(dir, realLibraryName()) {
		t.Errorf("Publisher got LibraryPath %q, want the resolved install dir's own library", fake.gotReq.LibraryPath)
	}
	if fake.gotReq.Title != testModName {
		t.Errorf("Publisher got Title %q, want %q", fake.gotReq.Title, testModName)
	}
	if info, err := os.Stat(fake.gotReq.ContentFolder); err != nil || !info.IsDir() {
		t.Errorf("Publisher got ContentFolder %q, want a real, existing directory", fake.gotReq.ContentFolder)
	}
}

func TestPublishModToWorkshopPassesExcludePathsThroughUnchanged(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	fake := &fakePublisher{result: workshop.PublishResult{PublishedFileID: 1}}
	a.workshopPublisher = fake

	req := WorkshopPublishRequest{ExcludePaths: []string{"common/notes.txt", "screenshots"}}
	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, req); err != nil {
		t.Fatalf("PublishModToWorkshop() error = %v", err)
	}
	if len(fake.gotReq.ExcludePaths) != 2 || fake.gotReq.ExcludePaths[0] != "common/notes.txt" || fake.gotReq.ExcludePaths[1] != "screenshots" {
		t.Errorf("Publisher got ExcludePaths %v, want [common/notes.txt screenshots]", fake.gotReq.ExcludePaths)
	}
}

func TestPublishModToWorkshopParsesAnExistingItemIDFromTheRequest(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	fake := &fakePublisher{result: workshop.PublishResult{PublishedFileID: 555}}
	a.workshopPublisher = fake

	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, WorkshopPublishRequest{ItemID: "555"}); err != nil {
		t.Fatalf("PublishModToWorkshop() error = %v", err)
	}
	if fake.gotReq.ItemID != 555 {
		t.Errorf("Publisher got ItemID %d, want 555", fake.gotReq.ItemID)
	}
}

func TestPublishModToWorkshopRejectsANonNumericItemID(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	a.workshopPublisher = &fakePublisher{}

	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, WorkshopPublishRequest{ItemID: "not-a-number"})
	if err == nil {
		t.Fatal("want an error for a non-numeric item id")
	}
}

func TestPublishModToWorkshopEmitsProgressEvents(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	a.workshopPublisher = &fakePublisher{result: workshop.PublishResult{PublishedFileID: 1}}

	var gotEvents []string
	a.eventSink = func(name string, data ...any) {
		gotEvents = append(gotEvents, name)
	}

	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, WorkshopPublishRequest{}); err != nil {
		t.Fatalf("PublishModToWorkshop() error = %v", err)
	}
	found := false
	for _, e := range gotEvents {
		if e == "workshop-publish-progress" {
			found = true
		}
	}
	if !found {
		t.Errorf("events emitted = %v, want at least one \"workshop-publish-progress\"", gotEvents)
	}
}

func TestPublishModToWorkshopPropagatesThePublishersOwnError(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	a.workshopPublisher = &fakePublisher{err: context.DeadlineExceeded}

	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want the fake publisher's own error to propagate")
	}
}
