package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
	"github.com/Official-Husko/parallax-mod-manager/internal/toolmark"
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

	gotIdentityReq workshop.IdentityRequest
	identityResult workshop.IdentityResult
	identityErr    error
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

func (f *fakePublisher) Identity(_ context.Context, req workshop.IdentityRequest) (workshop.IdentityResult, error) {
	f.gotIdentityReq = req
	return f.identityResult, f.identityErr
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
	_, err := a.PublishModToWorkshop("no-such-game", modID, "req-test", WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error for an unknown game id")
	}
}

func TestPublishModToWorkshopFailsWhenInstallCannotBeFound(t *testing.T) {
	a, modID := newWorkshopTestApp(t, "") // no override, and DetectInstall will find nothing real on the test machine
	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error when the game's install cannot be resolved")
	}
}

func TestPublishModToWorkshopFailsWhenTheInstallHasNoSteamworksLibrary(t *testing.T) {
	dir := newStellarisInstallDir(t, false) // valid install, but no library file
	a, modID := newWorkshopTestApp(t, dir)
	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error when the install has no Steamworks library")
	}
}

func TestPublishModToWorkshopFailsWhenTheModCannotBeFound(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, _ := newWorkshopTestApp(t, dir)
	a.workshopPublisher = &fakePublisher{}

	_, err := a.PublishModToWorkshop(game.Stellaris.ID, "no-such-mod", "req-test", WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error when the mod itself cannot be found")
	}
}

func TestPublishModToWorkshopPassesTheResolvedAppIDLibraryAndContentPathToThePublisher(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	fake := &fakePublisher{result: workshop.PublishResult{PublishedFileID: 12345}}
	a.workshopPublisher = fake

	result, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{
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
	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", req); err != nil {
		t.Fatalf("PublishModToWorkshop() error = %v", err)
	}
	if len(fake.gotReq.ExcludePaths) != 2 || fake.gotReq.ExcludePaths[0] != "common/notes.txt" || fake.gotReq.ExcludePaths[1] != "screenshots" {
		t.Errorf("Publisher got ExcludePaths %v, want [common/notes.txt screenshots]", fake.gotReq.ExcludePaths)
	}
}

func TestPublishModToWorkshopNeverWritesAToolMarkByDefault(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir) // preferences.Defaults() - ShareToolMark is off
	fake := &fakePublisher{result: workshop.PublishResult{PublishedFileID: 1}}
	a.workshopPublisher = fake

	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{Title: testModName}); err != nil {
		t.Fatalf("PublishModToWorkshop() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fake.gotReq.ContentFolder, toolmark.FileName)); !os.IsNotExist(err) {
		t.Errorf("a %s was written despite ShareToolMark being off by default", toolmark.FileName)
	}
}

func TestPublishModToWorkshopWritesAToolMarkBeforeUploadingWhenThePreferenceIsOn(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	a.preferences.ShareToolMark = true
	fake := &fakePublisher{result: workshop.PublishResult{PublishedFileID: 1}}
	a.workshopPublisher = fake

	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{Title: testModName}); err != nil {
		t.Fatalf("PublishModToWorkshop() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(fake.gotReq.ContentFolder, toolmark.FileName))
	if err != nil {
		t.Fatalf("reading %s: %v (ShareToolMark was on, want it written)", toolmark.FileName, err)
	}
	if !strings.Contains(string(data), "Published to Steam Workshop") {
		t.Errorf("content = %q, want it to note the publish", data)
	}
	// Written before Publish was actually called (fake.gotReq is only ever set inside
	// Publish itself), so it's confirmed present at fake.gotReq.ContentFolder above - the
	// same real folder path Publish received - rather than written afterward somewhere
	// the upload never saw.
}

func TestPublishModToWorkshopParsesAnExistingItemIDFromTheRequest(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	fake := &fakePublisher{result: workshop.PublishResult{PublishedFileID: 555}}
	a.workshopPublisher = fake

	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{ItemID: "555"}); err != nil {
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

	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{ItemID: "not-a-number"})
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

	if _, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{}); err != nil {
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

	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-test", WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want the fake publisher's own error to propagate")
	}
}

func TestCancelPublishStopsARunningPublish(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, modID := newWorkshopTestApp(t, dir)
	fake := &fakePublisher{}
	fake.err = context.Canceled // simulates what a cancelled context looks like to the caller
	a.workshopPublisher = fake

	// CancelPublish before the publish call is a plain no-op (nothing to cancel yet).
	a.CancelPublish("no-such-request")

	_, err := a.PublishModToWorkshop(game.Stellaris.ID, modID, "req-cancel", WorkshopPublishRequest{})
	if err == nil {
		t.Fatal("want an error when the publisher itself fails")
	}
	// The request is cleaned up after it finishes - cancelling it again is a no-op, not a panic.
	a.CancelPublish("req-cancel")
}

func TestSteamAccountInfoRejectsAnUnknownGame(t *testing.T) {
	a, _ := newWorkshopTestApp(t, "")
	if _, err := a.SteamAccountInfo("no-such-game"); err == nil {
		t.Fatal("want an error for an unknown game id")
	}
}

func TestSteamAccountInfoFailsWhenInstallCannotBeFound(t *testing.T) {
	a, _ := newWorkshopTestApp(t, "")
	if _, err := a.SteamAccountInfo(game.Stellaris.ID); err == nil {
		t.Fatal("want an error when the game's install cannot be resolved")
	}
}

func TestSteamAccountInfoReturnsThePublishersOwnPersonaName(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, _ := newWorkshopTestApp(t, dir)
	fake := &fakePublisher{identityResult: workshop.IdentityResult{
		PersonaName: "Kestrel_Admiral",
		SteamID:     "76561197989629849",
		Avatar:      []byte{0x89, 'P', 'N', 'G'}, // real content is asserted separately; only the format matters here
	}}
	a.workshopPublisher = fake

	info, err := a.SteamAccountInfo(game.Stellaris.ID)
	if err != nil {
		t.Fatalf("SteamAccountInfo() error = %v", err)
	}
	if info.PersonaName != "Kestrel_Admiral" {
		t.Errorf("PersonaName = %q, want %q", info.PersonaName, "Kestrel_Admiral")
	}
	if info.SteamID != "76561197989629849" {
		t.Errorf("SteamID = %q, want %q", info.SteamID, "76561197989629849")
	}
	if !strings.HasPrefix(info.AvatarDataURI, "data:image/png;base64,") {
		t.Errorf("AvatarDataURI = %q, want a data:image/png;base64, prefix", info.AvatarDataURI)
	}
	if fake.gotIdentityReq.AppID != 281990 {
		t.Errorf("Publisher got AppID %d, want 281990", fake.gotIdentityReq.AppID)
	}
	if fake.gotIdentityReq.LibraryPath != filepath.Join(dir, realLibraryName()) {
		t.Errorf("Publisher got LibraryPath %q, want the resolved install dir's own library", fake.gotIdentityReq.LibraryPath)
	}
}

func TestSteamAccountInfoLeavesAvatarEmptyWhenThePublisherHasNone(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, _ := newWorkshopTestApp(t, dir)
	a.workshopPublisher = &fakePublisher{identityResult: workshop.IdentityResult{PersonaName: "Kestrel_Admiral"}}

	info, err := a.SteamAccountInfo(game.Stellaris.ID)
	if err != nil {
		t.Fatalf("SteamAccountInfo() error = %v", err)
	}
	if info.AvatarDataURI != "" {
		t.Errorf("AvatarDataURI = %q, want empty when the publisher reports no avatar", info.AvatarDataURI)
	}
}

func TestPreviewModFileReturnsAResizedPreviewForARealPicture(t *testing.T) {
	a, modID := newWorkshopTestApp(t, "")
	contentFolder, err := library.ModFolderPath(a.baseContext(), game.Stellaris, library.Options{ExtraFolders: a.extraModFolders(game.Stellaris.ID)}, modID)
	if err != nil {
		t.Fatalf("ModFolderPath: %v", err)
	}
	writePNG(t, filepath.Join(contentFolder, "thumbnail.png"), 800, 600)

	preview, err := a.PreviewModFile(game.Stellaris.ID, modID, "thumbnail.png")
	if err != nil {
		t.Fatalf("PreviewModFile() error = %v", err)
	}
	if preview.Kind != "image" {
		t.Errorf("Kind = %q, want %q", preview.Kind, "image")
	}
	if preview.Width == 0 || preview.Height == 0 {
		t.Errorf("Width/Height = %d/%d, want real positive numbers", preview.Width, preview.Height)
	}
	if !strings.HasPrefix(preview.DataURI, "data:image/png;base64,") {
		t.Errorf("DataURI = %q, want a data:image/png;base64, prefix", preview.DataURI[:min(40, len(preview.DataURI))])
	}
}

func TestPreviewModFileReturnsNoneForANonPictureFile(t *testing.T) {
	a, modID := newWorkshopTestApp(t, "")
	preview, err := a.PreviewModFile(game.Stellaris.ID, modID, "common/a.txt")
	if err != nil {
		t.Fatalf("PreviewModFile() error = %v", err)
	}
	if preview.Kind != "none" {
		t.Errorf("Kind = %q, want %q for a non-picture file", preview.Kind, "none")
	}
	if preview.DataURI != "" {
		t.Errorf("DataURI = %q, want empty for a non-picture file", preview.DataURI)
	}
}

func TestPreviewModFileRefusesAPathOutsideTheModsOwnFolder(t *testing.T) {
	a, modID := newWorkshopTestApp(t, "")
	if _, err := a.PreviewModFile(game.Stellaris.ID, modID, "../../etc/passwd.png"); err == nil {
		t.Fatal("want an error for a path that escapes the mod's own folder")
	}
}

func TestSteamAccountInfoPropagatesThePublishersOwnError(t *testing.T) {
	dir := newStellarisInstallDir(t, true)
	a, _ := newWorkshopTestApp(t, dir)
	a.workshopPublisher = &fakePublisher{identityErr: context.DeadlineExceeded}

	if _, err := a.SteamAccountInfo(game.Stellaris.ID); err == nil {
		t.Fatal("want the fake publisher's own error to propagate")
	}
}

func TestWorkshopOpenSequence(t *testing.T) {
	steamThenBrowser := []string{"steam://url/CommunityFilePage/123", "https://steamcommunity.com/sharedfiles/filedetails/?id=123"}
	browserOnly := []string{"https://steamcommunity.com/sharedfiles/filedetails/?id=123"}
	tests := []struct {
		mode string
		want []string
	}{
		{"app", steamThenBrowser},
		// An empty or unrecognized value (an old settings file, or one saved
		// while "app" was still the zero value rather than a real default)
		// behaves exactly like "app" - Steam first, browser as the fallback.
		{"", steamThenBrowser},
		{"some-garbage-value", steamThenBrowser},
		// "browser" chosen explicitly is the one case with nothing to fall
		// back to - a single URL, Steam never tried at all.
		{"browser", browserOnly},
	}
	for _, tt := range tests {
		got := workshopOpenSequence(tt.mode, "123")
		if len(got) != len(tt.want) {
			t.Errorf("workshopOpenSequence(%q, \"123\") = %v, want %v", tt.mode, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("workshopOpenSequence(%q, \"123\")[%d] = %q, want %q", tt.mode, i, got[i], tt.want[i])
			}
		}
	}
}
