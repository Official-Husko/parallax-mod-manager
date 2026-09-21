package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/backgrounds"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
)

// How long a listing of the published images is trusted without asking GitHub
// again. Unauthenticated requests are limited to 60 an hour, and the listing
// rarely changes; past this it is revalidated with an ETag, which costs nothing
// against the limit when nothing changed.
const manifestMaxAge = time.Hour

// An empty listing (nothing published yet) is trusted for much less: it is the
// state someone is about to change by publishing, and should not stay in effect
// for an hour after they have.
const emptyManifestMaxAge = 5 * time.Minute

func manifestFreshFor(m backgrounds.Manifest) time.Duration {
	if len(m.Packs) == 0 {
		return emptyManifestMaxAge
	}
	return manifestMaxAge
}

// backgroundState is everything the background art keeps between calls. The zero
// value works once initBackgrounds has set the paths.
type backgroundState struct {
	source  backgrounds.Source
	store   backgrounds.Store
	fetcher backgrounds.Fetcher
	cache   backgrounds.ManifestCache

	mu        sync.Mutex
	manifest  *backgrounds.Manifest
	etag      string
	fetchedAt time.Time

	dlMu     sync.Mutex
	dlCancel context.CancelFunc
}

// initBackgrounds wires the background art up once the config and cache folders
// are known: the source (built in, or the user's override), the offline store and
// the listing cache.
func (a *App) initBackgrounds(configAppDir string) {
	bg := &a.backgrounds
	log := applog.For("Backgrounds")
	var override []byte
	if data, err := os.ReadFile(filepath.Join(configAppDir, "backgrounds.jsonc")); err == nil {
		override = data
	}
	src, notice, err := backgrounds.LoadSource(embeddedBackgroundSource, override)
	if err != nil {
		log.Errorf("%v", err)
	}
	if notice != "" {
		log.Warnf("%s", notice)
	}
	bg.source = src
	bg.store = backgrounds.Store{Dir: filepath.Join(configAppDir, "game_media", "backgrounds")}
	bg.fetcher = backgrounds.Fetcher{UserAgent: "parallax-mod-manager/" + AppVersion}
	if a.cacheDir != "" {
		bg.cache = backgrounds.ManifestCache{Path: filepath.Join(a.cacheDir, "backgrounds_manifest.json")}
	}
}

// backgroundMiddleware serves the offline images on the app's own asset route.
func (a *App) backgroundMiddleware(next http.Handler) http.Handler {
	return backgrounds.Middleware(func() backgrounds.Store { return a.backgrounds.store })(next)
}

// backgroundManifest returns what is published, from memory, the disk cache or
// GitHub (in that order of preference, refetching when older than manifestMaxAge
// or when force is set). When GitHub cannot be reached it returns whatever it
// already had, with the reason - an offline start still gets the last known list.
func (a *App) backgroundManifest(force bool) (backgrounds.Manifest, string) {
	bg := &a.backgrounds
	bg.mu.Lock()
	defer bg.mu.Unlock()

	if bg.manifest == nil {
		if cached, ok := bg.cache.Load(); ok && cached.Source == bg.source {
			m := cached.Manifest
			bg.manifest, bg.etag, bg.fetchedAt = &m, cached.ETag, time.Unix(cached.FetchedAt, 0)
		}
	}
	if bg.manifest != nil && !force && time.Since(bg.fetchedAt) < manifestFreshFor(*bg.manifest) {
		return *bg.manifest, ""
	}

	log := applog.For("Backgrounds")
	timer := log.Begin()
	m, etag, notModified, err := bg.fetcher.Fetch(a.baseContext(), bg.source, bg.etag)
	if err != nil {
		timer.Warnf("couldn't list the published backgrounds: %v", err)
		if bg.manifest != nil {
			return *bg.manifest, err.Error()
		}
		return backgrounds.Manifest{}, err.Error()
	}
	if notModified && bg.manifest != nil {
		m = *bg.manifest
		timer.Infof("background listing unchanged")
	} else {
		timer.Infof("listed %d game%s of published backgrounds", len(m.Packs), plural(len(m.Packs)))
	}
	bg.manifest, bg.etag, bg.fetchedAt = &m, etag, time.Now()
	if err := bg.cache.Save(backgrounds.CachedManifest{Source: bg.source, ETag: etag, FetchedAt: bg.fetchedAt.Unix(), Manifest: m}); err != nil && bg.cache.Path != "" {
		log.Warnf("couldn't cache the background listing: %v", err)
	}
	return m, ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// BackgroundPack is one game's backgrounds as the download window shows them:
// what is published, what is already on disk, and what is left to fetch.
type BackgroundPack struct {
	GameID string
	// Files and Bytes are what is published (Bytes is approximate only in that it
	// is the repository's own listing, not a measured download).
	Files int
	Bytes int64
	// LocalFiles and LocalBytes are what is on disk now, whatever put it there.
	LocalFiles int
	LocalBytes int64
	// MissingFiles and MissingBytes are the published images not on disk with the
	// right size: what choosing this game would download.
	MissingFiles int
	MissingBytes int64
}

// BackgroundCatalog is the download window's data.
type BackgroundCatalog struct {
	// Packs has every game that has published images or images on disk, by id.
	Packs []BackgroundPack
	// RemoteError is why the published list could not be read ("" when it was);
	// packs then show only what is on disk.
	RemoteError string
	// Folder is where offline images are kept - and where a user can drop their
	// own, in a subfolder named by the game's id.
	Folder string
	// Source is the repository the images are published in, "owner/name".
	Source string
}

// BackgroundCatalog lists what backgrounds exist for each game, published and on
// disk. refresh asks GitHub again instead of using the listing it has.
func (a *App) BackgroundCatalog(refresh bool) BackgroundCatalog {
	bg := &a.backgrounds
	manifest, remoteErr := a.backgroundManifest(refresh)

	byGame := map[string]*BackgroundPack{}
	pack := func(id string) *BackgroundPack {
		if byGame[id] == nil {
			byGame[id] = &BackgroundPack{GameID: id}
		}
		return byGame[id]
	}
	for _, p := range manifest.Packs {
		bp := pack(p.GameID)
		bp.Files, bp.Bytes = len(p.Files), p.Bytes
		for _, f := range p.Files {
			if !bg.store.Has(p.GameID, f) {
				bp.MissingFiles++
				bp.MissingBytes += f.Size
			}
		}
	}
	for _, id := range bg.store.Games() {
		bp := pack(id)
		bp.LocalFiles, bp.LocalBytes = bg.store.Usage(id)
	}
	packs := make([]BackgroundPack, 0, len(byGame))
	for _, bp := range byGame {
		if bp.Files == 0 && bp.LocalFiles == 0 {
			continue
		}
		packs = append(packs, *bp)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].GameID < packs[j].GameID })
	return BackgroundCatalog{Packs: packs, RemoteError: remoteErr, Folder: bg.store.Dir, Source: bg.source.Repo}
}

// BackgroundImages returns the addresses of gameID's background images for the UI
// to rotate through: in online mode the published ones, streamed from GitHub; in
// offline mode - or online with nothing published or GitHub unreachable - the ones
// on disk, served by the app itself. Empty means no background for this game.
func (a *App) BackgroundImages(gameID string) []string {
	if !backgrounds.ValidGameID(gameID) {
		return []string{}
	}
	urls, origin := a.backgroundImages(gameID)
	log := applog.For("Backgrounds")
	if len(urls) == 0 {
		log.Infof("no background images for '%s' (%s)", a.backgroundGameLabel(gameID), origin)
	} else {
		log.Infof("'%s': %d background image%s %s", a.backgroundGameLabel(gameID), len(urls), plural(len(urls)), origin)
	}
	return urls
}

// backgroundImages is BackgroundImages' choice, with where the answer came from in
// words for the activity log - in particular why an online setting ended up on the
// offline copies.
func (a *App) backgroundImages(gameID string) ([]string, string) {
	bg := &a.backgrounds
	local := func() []string {
		files := bg.store.List(gameID)
		urls := make([]string, 0, len(files))
		for _, f := range files {
			urls = append(urls, backgrounds.LocalURL(gameID, f.Name))
		}
		return urls
	}
	if a.GetPreferences().BackgroundSource == preferences.BackgroundSourceOffline {
		return local(), "from this computer (offline mode)"
	}
	manifest, remoteErr := a.backgroundManifest(false)
	if p, ok := manifest.Pack(gameID); ok && len(p.Files) > 0 {
		urls := make([]string, 0, len(p.Files))
		for _, f := range p.Files {
			urls = append(urls, bg.source.RawURL(bg.fetcher.Raw(), gameID, f.Name))
		}
		return urls, "from GitHub (online mode)"
	}
	if remoteErr != "" {
		return local(), "from this computer, GitHub could not be reached: " + remoteErr
	}
	return local(), "from this computer, none are published for it"
}

// backgroundGameLabel is a game's name for the log, or its id when unknown.
func (a *App) backgroundGameLabel(gameID string) string {
	if a.registry != nil {
		if cfg, ok := a.registry.Get(gameID); ok {
			return cfg.DisplayName
		}
	}
	return gameID
}

// StartBackgroundDownload downloads the published backgrounds for gameIDs into the
// offline folder, in the background. Progress arrives as "background-download"
// events (see backgrounds.Progress) - the last has state "done" or "cancelled" -
// and "background-packs-changed" follows when it ends. What is already on disk
// is skipped, so it can be run again to resume or top up.
func (a *App) StartBackgroundDownload(gameIDs []string) error {
	bg := &a.backgrounds
	bg.dlMu.Lock()
	defer bg.dlMu.Unlock()
	if bg.dlCancel != nil {
		return fmt.Errorf("a background download is already running")
	}

	manifest, remoteErr := a.backgroundManifest(false)
	var jobs []backgrounds.Job
	for _, id := range gameIDs {
		if !backgrounds.ValidGameID(id) {
			return fmt.Errorf("%q is not a valid game id", id)
		}
		if p, ok := manifest.Pack(id); ok {
			jobs = append(jobs, backgrounds.Job{GameID: id, Files: p.Files})
		}
	}
	if len(jobs) == 0 {
		if remoteErr != "" {
			return fmt.Errorf("couldn't list the published backgrounds: %s", remoteErr)
		}
		return fmt.Errorf("nothing is published for the chosen games")
	}

	ctx, cancel := context.WithCancel(a.baseContext())
	bg.dlCancel = cancel
	log := applog.For("Backgrounds")
	progress := newDownloadLog(a, jobs, log)
	d := backgrounds.Downloader{
		Store:     bg.store,
		URL:       func(gameID, name string) string { return bg.source.RawURL(bg.fetcher.Raw(), gameID, name) },
		UserAgent: bg.fetcher.UserAgent,
		OnFile:    progress.file,
	}
	progress.announce()
	go func() {
		timer := log.Begin()
		res, err := d.Run(ctx, jobs, func(p backgrounds.Progress) { a.emit("background-download", p) })
		bg.dlMu.Lock()
		bg.dlCancel = nil
		bg.dlMu.Unlock()
		cancel()
		switch {
		case errors.Is(err, context.Canceled):
			timer.Infof("background download cancelled after %d image%s (%s); what was finished is kept", res.Downloaded, plural(res.Downloaded), formatSize(res.Bytes))
		case err != nil:
			timer.Warnf("background download stopped after %d image%s: %v", res.Downloaded, plural(res.Downloaded), err)
		case res.Failed > 0:
			timer.Warnf("background download finished: %d image%s (%s) downloaded, %d failed - run it again to retry them", res.Downloaded, plural(res.Downloaded), formatSize(res.Bytes), res.Failed)
		default:
			timer.Infof("background download finished: %d image%s (%s) downloaded, %d already on disk", res.Downloaded, plural(res.Downloaded), formatSize(res.Bytes), res.Skipped)
		}
		a.emit("background-packs-changed")
	}()
	return nil
}

// downloadLog turns a download's per-image results into activity log lines: what
// is about to be fetched, each image (debug level - a big pack is hundreds of
// lines), every failure with its reason (the first few as warnings, the rest as
// debug so a dead connection does not bury the log), and one line as each game's
// images are done.
type downloadLog struct {
	app *App
	log applog.Logger

	mu       sync.Mutex
	games    map[string]*gameDownload
	order    []string
	skipped  int
	toFetch  int
	bytes    int64
	warnings int
}

type gameDownload struct {
	total, remaining, ok, failed int
	bytes                        int64
}

// maxDownloadWarnings is how many failed images are logged as warnings.
const maxDownloadWarnings = 10

func newDownloadLog(a *App, jobs []backgrounds.Job, log applog.Logger) *downloadLog {
	dl := &downloadLog{app: a, log: log, games: map[string]*gameDownload{}}
	for _, j := range jobs {
		g := &gameDownload{}
		for _, f := range j.Files {
			if backgrounds.ValidName(f.Name) && !a.backgrounds.store.Has(j.GameID, f) {
				g.total++
				g.remaining++
				dl.toFetch++
				dl.bytes += f.Size
			} else if backgrounds.ValidName(f.Name) {
				dl.skipped++
			}
		}
		dl.games[j.GameID] = g
		dl.order = append(dl.order, j.GameID)
	}
	return dl
}

// announce logs what the download is about to do.
func (dl *downloadLog) announce() {
	names := make([]string, 0, len(dl.order))
	for _, id := range dl.order {
		names = append(names, "'"+dl.app.backgroundGameLabel(id)+"'")
	}
	list := strings.Join(names, ", ")
	if dl.toFetch == 0 {
		dl.log.Infof("background images for %s are already on disk (%d image%s), nothing to download", list, dl.skipped, plural(dl.skipped))
		return
	}
	dl.log.Infof("downloading %d background image%s (~%s) for %s; %d already on disk", dl.toFetch, plural(dl.toFetch), formatSize(dl.bytes), list, dl.skipped)
}

// file is the downloader's per-image callback (called from several goroutines).
func (dl *downloadLog) file(r backgrounds.FileResult) {
	dl.mu.Lock()
	g := dl.games[r.GameID]
	warn := false
	var done gameDownload
	finished := false
	if g != nil {
		g.remaining--
		if r.Err == nil {
			g.ok++
			g.bytes += r.Size
		} else {
			g.failed++
			if dl.warnings < maxDownloadWarnings {
				dl.warnings++
				warn = true
			}
		}
		if g.remaining == 0 {
			finished, done = true, *g
		}
	}
	dl.mu.Unlock()

	label := dl.app.backgroundGameLabel(r.GameID)
	switch {
	case r.Err == nil:
		dl.log.Debugf("downloaded '%s' of '%s' (%s in %d ms)", r.Name, label, formatSize(r.Size), r.Took.Milliseconds())
	case warn:
		dl.log.Warnf("couldn't download '%s' of '%s': %v", r.Name, label, r.Err)
	default:
		dl.log.Debugf("couldn't download '%s' of '%s': %v", r.Name, label, r.Err)
	}
	if finished {
		if done.failed > 0 {
			dl.log.Warnf("'%s': %d of %d images downloaded (%s), %d failed", label, done.ok, done.total, formatSize(done.bytes), done.failed)
		} else {
			dl.log.Infof("'%s': all %d images downloaded (%s)", label, done.ok, formatSize(done.bytes))
		}
	}
}

// formatSize renders a byte count for the log: "512 B", "557.8 KB", "84.5 MB".
func formatSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// CancelBackgroundDownload stops a running download; what it had finished stays.
func (a *App) CancelBackgroundDownload() {
	bg := &a.backgrounds
	bg.dlMu.Lock()
	cancel := bg.dlCancel
	bg.dlMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// RemoveBackgroundPack deletes gameID's offline images. Refused while a download
// runs, which could be writing into that folder.
func (a *App) RemoveBackgroundPack(gameID string) error {
	bg := &a.backgrounds
	bg.dlMu.Lock()
	running := bg.dlCancel != nil
	bg.dlMu.Unlock()
	if running {
		return fmt.Errorf("a background download is running - cancel it first")
	}
	if err := bg.store.RemovePack(gameID); err != nil {
		return err
	}
	applog.For("Backgrounds").Infof("removed the offline backgrounds for game %s", gameID)
	a.emit("background-packs-changed")
	return nil
}
