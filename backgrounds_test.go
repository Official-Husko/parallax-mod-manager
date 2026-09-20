package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/backgrounds"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// githubFake stands in for GitHub: the tree listing at the API path and the images
// at the raw path. Both are served from one server.
type githubFake struct {
	server   *httptest.Server
	treeHits int32
	imgHits  int32
	etag     string
	files    map[string]string // "gameID/name" -> content
	gate     chan struct{}     // when non-nil, image requests wait for it
	down     atomic.Bool
}

const fakeTreePath = "/repos/o/r/git/trees/main:game_media/backgrounds"

func newGithubFake(t *testing.T, files map[string]string) *githubFake {
	t.Helper()
	g := &githubFake{files: files, etag: `"v1"`}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch {
		case r.URL.Path == fakeTreePath:
			atomic.AddInt32(&g.treeHits, 1)
			if r.Header.Get("If-None-Match") == g.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", g.etag)
			var b strings.Builder
			b.WriteString(`{"truncated":false,"tree":[`)
			first := true
			for path, body := range g.files {
				if !first {
					b.WriteString(",")
				}
				first = false
				b.WriteString(`{"path":"` + path + `","type":"blob","size":` + itoa(len(body)) + `}`)
			}
			b.WriteString(`]}`)
			w.Write([]byte(b.String()))
		case strings.HasPrefix(r.URL.Path, "/o/r/main/game_media/backgrounds/"):
			atomic.AddInt32(&g.imgHits, 1)
			if g.gate != nil {
				select {
				case <-g.gate:
				case <-r.Context().Done():
					return
				}
			}
			body, ok := g.files[strings.TrimPrefix(r.URL.Path, "/o/r/main/game_media/backgrounds/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.server.Close)
	return g
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	return string(d)
}

type recordedEvents struct {
	mu     sync.Mutex
	events []recordedEvent
}

type recordedEvent struct {
	name string
	data []any
}

func (r *recordedEvents) sink(name string, data ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedEvent{name, data})
}

func (r *recordedEvents) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.name == name {
			n++
		}
	}
	return n
}

func (r *recordedEvents) lastProgress() (backgrounds.Progress, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.events) - 1; i >= 0; i-- {
		if r.events[i].name == "background-download" {
			return r.events[i].data[0].(backgrounds.Progress), true
		}
	}
	return backgrounds.Progress{}, false
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// backgroundsApp is an App whose backgrounds talk to fake and keep their files in
// temp folders. Calling it again with the same dirs is a restart.
func backgroundsApp(t *testing.T, fake *githubFake, configDir, cacheDir string) (*App, *recordedEvents) {
	t.Helper()
	events := &recordedEvents{}
	a := &App{ctx: context.Background(), eventSink: events.sink, preferences: preferences.Defaults()}
	bg := &a.backgrounds
	bg.source = backgrounds.Source{Repo: "o/r", Branch: "main", Path: "game_media/backgrounds"}
	bg.store = backgrounds.Store{Dir: filepath.Join(configDir, "game_media", "backgrounds")}
	bg.fetcher = backgrounds.Fetcher{APIBase: fake.server.URL, RawBase: fake.server.URL, UserAgent: "test"}
	bg.cache = backgrounds.ManifestCache{Path: filepath.Join(cacheDir, "backgrounds_manifest.json")}
	return a, events
}

func TestTheBuiltInBackgroundSourceIsValid(t *testing.T) {
	src, notice, err := backgrounds.LoadSource(embeddedBackgroundSource, nil)
	if err != nil || notice != "" {
		t.Fatalf("err = %v, notice = %q", err, notice)
	}
	if src.Repo != "Official-Husko/parallax-mod-manager" || src.Branch == "" || src.Path == "" {
		t.Errorf("src = %+v", src)
	}
}

func TestBackgroundSourcePreferenceDefaultsToOnlineAndIsNormalized(t *testing.T) {
	if got := preferences.Defaults().BackgroundSource; got != "online" {
		t.Errorf("default = %q, want online", got)
	}
	for in, want := range map[string]string{"": "online", "online": "online", "offline": "offline", "OFFLINE": "online", "nonsense": "online"} {
		if got := preferences.NormalizedBackgroundSource(in); got != want {
			t.Errorf("Normalized(%q) = %q, want %q", in, got, want)
		}
	}
	a := &App{}
	p := preferences.Defaults()
	p.BackgroundSource = "garbage"
	if err := a.SetPreferences(p); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}
	if got := a.GetPreferences().BackgroundSource; got != "online" {
		t.Errorf("saved %q, want a bad value stored as online", got)
	}
}

func TestBackgroundCatalogShowsSizesAndWhatIsAlreadyOnDisk(t *testing.T) {
	fake := newGithubFake(t, map[string]string{"g1/a.png": strings.Repeat("A", 1000), "g1/b.jpg": strings.Repeat("B", 3000), "g2/c.webp": "CCCC"})
	a, _ := backgroundsApp(t, fake, t.TempDir(), t.TempDir())
	dir := a.backgrounds.store.Dir
	if err := os.MkdirAll(filepath.Join(dir, "g1"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "g1", "a.png"), []byte(strings.Repeat("A", 1000)), 0o644) // already downloaded
	os.MkdirAll(filepath.Join(dir, "own"), 0o755)
	os.WriteFile(filepath.Join(dir, "own", "mine.jpg"), []byte("hand placed"), 0o644) // the user's own, not published

	cat := a.BackgroundCatalog(false)
	if cat.RemoteError != "" || cat.Folder != dir || cat.Source != "o/r" {
		t.Fatalf("catalog = %+v", cat)
	}
	byID := map[string]BackgroundPack{}
	for _, p := range cat.Packs {
		byID[p.GameID] = p
	}
	if len(byID) != 3 {
		t.Fatalf("packs = %+v, want g1, g2 and the user's own folder", cat.Packs)
	}
	g1 := byID["g1"]
	if g1.Files != 2 || g1.Bytes != 4000 || g1.LocalFiles != 1 || g1.LocalBytes != 1000 || g1.MissingFiles != 1 || g1.MissingBytes != 3000 {
		t.Errorf("g1 = %+v", g1)
	}
	if g2 := byID["g2"]; g2.MissingFiles != 1 || g2.MissingBytes != 4 || g2.LocalFiles != 0 {
		t.Errorf("g2 = %+v", g2)
	}
	if own := byID["own"]; own.Files != 0 || own.LocalFiles != 1 || own.MissingFiles != 0 {
		t.Errorf("own = %+v, want only what is on disk", own)
	}
}

func TestBackgroundImagesOnlineStreamsFromGitHubAndOfflineUsesTheDisk(t *testing.T) {
	fake := newGithubFake(t, map[string]string{"g1/a b.png": "AAAA", "g1/c.jpg": "CC"})
	a, _ := backgroundsApp(t, fake, t.TempDir(), t.TempDir())
	os.MkdirAll(filepath.Join(a.backgrounds.store.Dir, "g1"), 0o755)
	os.WriteFile(filepath.Join(a.backgrounds.store.Dir, "g1", "local.png"), []byte("L"), 0o644)

	online := a.BackgroundImages("g1")
	want := []string{fake.server.URL + "/o/r/main/game_media/backgrounds/g1/a%20b.png", fake.server.URL + "/o/r/main/game_media/backgrounds/g1/c.jpg"}
	if len(online) != 2 || online[0] != want[0] || online[1] != want[1] {
		t.Errorf("online = %v, want %v", online, want)
	}

	a.preferences.BackgroundSource = "offline"
	hitsBefore := atomic.LoadInt32(&fake.treeHits)
	offline := a.BackgroundImages("g1")
	if len(offline) != 1 || offline[0] != "/backgrounds/g1/local.png" {
		t.Errorf("offline = %v, want the one image on disk, served by the app", offline)
	}
	if atomic.LoadInt32(&fake.treeHits) != hitsBefore {
		t.Error("offline mode must not touch the network")
	}

	if got := a.BackgroundImages("../etc"); len(got) != 0 {
		t.Errorf("an unsafe game id gave %v", got)
	}
	if got := a.BackgroundImages("unknown-game"); got == nil || len(got) != 0 {
		t.Errorf("a game with nothing gave %#v, want an empty non-nil list", got)
	}
}

func TestBackgroundListingIsCachedAndRevalidatedWithTheETag(t *testing.T) {
	fake := newGithubFake(t, map[string]string{"g1/a.png": "A"})
	cfg, cache := t.TempDir(), t.TempDir()
	a, _ := backgroundsApp(t, fake, cfg, cache)

	a.BackgroundImages("g1")
	a.BackgroundImages("g1")
	a.BackgroundCatalog(false)
	if got := atomic.LoadInt32(&fake.treeHits); got != 1 {
		t.Errorf("listing requests = %d, want 1 (the rest served from memory)", got)
	}

	// Stale: asks again, with the ETag, and gets 304.
	a.backgrounds.mu.Lock()
	a.backgrounds.fetchedAt = time.Now().Add(-2 * time.Hour)
	a.backgrounds.mu.Unlock()
	if imgs := a.BackgroundImages("g1"); len(imgs) != 1 {
		t.Errorf("images after revalidation = %v", imgs)
	}
	if got := atomic.LoadInt32(&fake.treeHits); got != 2 {
		t.Errorf("listing requests = %d, want a second (conditional) one", got)
	}

	// A restart with GitHub down still knows what was published.
	fake.down.Store(true)
	restarted, _ := backgroundsApp(t, fake, cfg, cache)
	if imgs := restarted.BackgroundImages("g1"); len(imgs) != 1 {
		t.Errorf("after a restart with GitHub down: %v, want the cached listing", imgs)
	}
	cat := restarted.BackgroundCatalog(true) // forced refresh fails, cached list stays
	if cat.RemoteError == "" || len(cat.Packs) != 1 {
		t.Errorf("catalog = %+v, want the cached pack and the reason it could not refresh", cat)
	}
}

func TestBackgroundImagesFallBackToTheDiskWhenGitHubCannotBeReached(t *testing.T) {
	fake := newGithubFake(t, map[string]string{"g1/a.png": "A"})
	fake.down.Store(true)
	a, _ := backgroundsApp(t, fake, t.TempDir(), t.TempDir())
	os.MkdirAll(filepath.Join(a.backgrounds.store.Dir, "g1"), 0o755)
	os.WriteFile(filepath.Join(a.backgrounds.store.Dir, "g1", "kept.jpg"), []byte("K"), 0o644)

	got := a.BackgroundImages("g1") // online, no network, no cache
	if len(got) != 1 || got[0] != "/backgrounds/g1/kept.jpg" {
		t.Errorf("got %v, want the offline copy rather than nothing", got)
	}
	if empty := a.BackgroundImages("g2"); len(empty) != 0 {
		t.Errorf("g2 = %v, want none", empty)
	}
	if err := a.StartBackgroundDownload([]string{"g1"}); err == nil || !strings.Contains(err.Error(), "couldn't list") {
		t.Errorf("StartBackgroundDownload without a listing: %v, want a clear reason", err)
	}
}

func TestBackgroundDownloadEndToEnd(t *testing.T) {
	files := map[string]string{"g1/a.png": strings.Repeat("A", 2000), "g1/b b.jpg": strings.Repeat("B", 500), "g2/z.webp": "ZZZ"}
	fake := newGithubFake(t, files)
	a, events := backgroundsApp(t, fake, t.TempDir(), t.TempDir())

	if err := a.StartBackgroundDownload([]string{"g1", "no-such-game"}); err != nil {
		t.Fatalf("StartBackgroundDownload: %v", err)
	}
	waitFor(t, "the download to finish", func() bool { return events.count("background-packs-changed") == 1 })

	last, ok := events.lastProgress()
	if !ok || last.State != "done" || last.FilesDone != 2 || last.FilesTotal != 2 || last.BytesDone != 2500 || last.BytesTotal != 2500 || last.Failed != 0 {
		t.Errorf("last progress = %+v", last)
	}
	for _, name := range []string{"a.png", "b b.jpg"} {
		data, err := os.ReadFile(filepath.Join(a.backgrounds.store.Dir, "g1", name))
		if err != nil || string(data) != files["g1/"+name] {
			t.Errorf("g1/%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(a.backgrounds.store.Dir, "g2")); err == nil {
		t.Error("g2 was not chosen and must not be downloaded")
	}

	// Offline now serves them; the catalog says nothing is missing for g1.
	a.preferences.BackgroundSource = "offline"
	if imgs := a.BackgroundImages("g1"); len(imgs) != 2 || imgs[0] != "/backgrounds/g1/a.png" || imgs[1] != "/backgrounds/g1/b%20b.jpg" {
		t.Errorf("offline images = %v", imgs)
	}
	for _, p := range a.BackgroundCatalog(false).Packs {
		if p.GameID == "g1" && (p.MissingFiles != 0 || p.LocalFiles != 2) {
			t.Errorf("g1 = %+v, want nothing left to download", p)
		}
	}

	// Running it again resumes: nothing is fetched twice.
	before := atomic.LoadInt32(&fake.imgHits)
	if err := a.StartBackgroundDownload([]string{"g1"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	waitFor(t, "the second run", func() bool { return events.count("background-packs-changed") == 2 })
	if after := atomic.LoadInt32(&fake.imgHits); after != before {
		t.Errorf("image requests went %d -> %d, want none for what is already on disk", before, after)
	}

	// Removing a pack deletes that game's folder only.
	if err := a.RemoveBackgroundPack("g1"); err != nil {
		t.Fatalf("RemoveBackgroundPack: %v", err)
	}
	if imgs := a.BackgroundImages("g1"); len(imgs) != 0 {
		t.Errorf("after removing: %v", imgs)
	}
	if events.count("background-packs-changed") != 3 {
		t.Error("removing a pack should announce the change")
	}
}

func TestBackgroundDownloadOnlyOneAtATimeAndCancel(t *testing.T) {
	fake := newGithubFake(t, map[string]string{"g1/a.png": "AAAA", "g1/b.png": "BBBB", "g1/c.png": "CCCC"})
	fake.gate = make(chan struct{})
	a, events := backgroundsApp(t, fake, t.TempDir(), t.TempDir())

	if err := a.StartBackgroundDownload([]string{"g1"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "images to be requested", func() bool { return atomic.LoadInt32(&fake.imgHits) >= 1 })
	if err := a.StartBackgroundDownload([]string{"g1"}); err == nil {
		t.Error("a second download while one runs must be refused")
	}
	if err := a.RemoveBackgroundPack("g1"); err == nil {
		t.Error("removing a pack while a download runs must be refused")
	}

	a.CancelBackgroundDownload()
	waitFor(t, "the cancelled download to end", func() bool { return events.count("background-packs-changed") == 1 })
	last, _ := events.lastProgress()
	if last.State != "cancelled" {
		t.Errorf("last progress = %+v, want cancelled", last)
	}
	filepath.Walk(a.backgrounds.store.Dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".part") {
			t.Errorf("leftover %s", p)
		}
		return nil
	})

	// After it ends, a new one may start (and the cancel of nothing is harmless).
	a.CancelBackgroundDownload()
	close(fake.gate)
	if err := a.StartBackgroundDownload([]string{"g1"}); err != nil {
		t.Errorf("a new download after the cancel: %v", err)
	}
	waitFor(t, "the resumed download", func() bool { return events.count("background-packs-changed") == 2 })
	if last, _ := events.lastProgress(); last.State != "done" || last.Failed != 0 {
		t.Errorf("resumed = %+v", last)
	}
}

func TestBackgroundMiddlewareServesTheAppsOwnStore(t *testing.T) {
	fake := newGithubFake(t, nil)
	a, _ := backgroundsApp(t, fake, t.TempDir(), t.TempDir())
	os.MkdirAll(filepath.Join(a.backgrounds.store.Dir, "g1"), 0o755)
	os.WriteFile(filepath.Join(a.backgrounds.store.Dir, "g1", "x y.png"), []byte("PNGDATA"), 0o644)

	h := a.backgroundMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, backgrounds.LocalURL("g1", "x y.png"), nil))
	if rec.Code != 200 || rec.Body.String() != "PNGDATA" {
		t.Errorf("served %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("other paths must reach the app's assets, got %d", rec.Code)
	}
}

func TestBackgroundListingCacheIsIgnoredWhenItCameFromAnotherSource(t *testing.T) {
	fake := newGithubFake(t, map[string]string{"g1/a.png": "A"})
	cfg, cache := t.TempDir(), t.TempDir()
	a, _ := backgroundsApp(t, fake, cfg, cache)

	// A cache written for somewhere else (the folder moved, or an older build that
	// recorded no source at all) describes the wrong place.
	stale := backgrounds.CachedManifest{
		Source:    backgrounds.Source{Repo: "o/r", Branch: "main", Path: "an/older/folder"},
		ETag:      `"old"`,
		FetchedAt: time.Now().Unix(),
		Manifest:  backgrounds.Manifest{Packs: []backgrounds.Pack{{GameID: "ghost", Files: []backgrounds.File{{Name: "x.png", Size: 1}}, Bytes: 1}}},
	}
	if err := a.backgrounds.cache.Save(stale); err != nil {
		t.Fatal(err)
	}
	imgs := a.BackgroundImages("g1")
	if len(imgs) != 1 || !strings.Contains(imgs[0], "/g1/a.png") {
		t.Errorf("images = %v, want the real listing, not the other source's cache", imgs)
	}
	if got := atomic.LoadInt32(&fake.treeHits); got != 1 {
		t.Errorf("listing requests = %d, want 1 (the mismatched cache must not count as fresh)", got)
	}
	if len(a.BackgroundImages("ghost")) != 0 {
		t.Error("the other source's pack leaked through")
	}

	// A cache with no source recorded at all (written before this field existed).
	oldFormat := `{"ETag":"","FetchedAt":` + itoa(int(time.Now().Unix())) + `,"Manifest":{"Packs":null}}`
	if err := os.WriteFile(a.backgrounds.cache.Path, []byte(oldFormat), 0o644); err != nil {
		t.Fatal(err)
	}
	restarted, _ := backgroundsApp(t, fake, cfg, cache)
	if imgs := restarted.BackgroundImages("g1"); len(imgs) != 1 {
		t.Errorf("after a restart over an old-format cache: %v, want the real listing", imgs)
	}
}

func TestAnEmptyListingIsRecheckedSoonSoPublishingShowsUpQuickly(t *testing.T) {
	fake := newGithubFake(t, map[string]string{})
	a, _ := backgroundsApp(t, fake, t.TempDir(), t.TempDir())

	if imgs := a.BackgroundImages("g1"); len(imgs) != 0 {
		t.Fatalf("nothing is published yet, got %v", imgs)
	}
	a.BackgroundImages("g1")
	if got := atomic.LoadInt32(&fake.treeHits); got != 1 {
		t.Errorf("listing requests = %d, want 1: an empty listing is still cached briefly", got)
	}

	// Someone publishes; 6 minutes later (past the short window for an empty list,
	// far inside the hour a real listing gets) it is noticed.
	fake.files["g1/a.png"] = "A"
	fake.etag = `"v2"`
	a.backgrounds.mu.Lock()
	a.backgrounds.fetchedAt = time.Now().Add(-6 * time.Minute)
	a.backgrounds.mu.Unlock()
	if imgs := a.BackgroundImages("g1"); len(imgs) != 1 {
		t.Errorf("after publishing: %v, want the new image", imgs)
	}

	// A non-empty listing keeps the long window: 6 minutes old is still fresh.
	hits := atomic.LoadInt32(&fake.treeHits)
	a.backgrounds.mu.Lock()
	a.backgrounds.fetchedAt = time.Now().Add(-6 * time.Minute)
	a.backgrounds.mu.Unlock()
	a.BackgroundImages("g1")
	if got := atomic.LoadInt32(&fake.treeHits); got != hits {
		t.Errorf("a real listing 6 minutes old was refetched (%d -> %d)", hits, got)
	}
}

func TestTheBuiltInBackgroundSourcePointsAtWhereTheImagesArePublished(t *testing.T) {
	src, _, err := backgrounds.LoadSource(embeddedBackgroundSource, nil)
	if err != nil {
		t.Fatal(err)
	}
	if src.Path != "frontend/src/assets/game_media/background" || src.Branch != "main" {
		t.Errorf("src = %+v, want the folder the images are published in", src)
	}
}
