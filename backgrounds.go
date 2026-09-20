package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
		if cached, ok := bg.cache.Load(); ok {
			m := cached.Manifest
			bg.manifest, bg.etag, bg.fetchedAt = &m, cached.ETag, time.Unix(cached.FetchedAt, 0)
		}
	}
	if bg.manifest != nil && !force && time.Since(bg.fetchedAt) < manifestMaxAge {
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
	if err := bg.cache.Save(backgrounds.CachedManifest{ETag: etag, FetchedAt: bg.fetchedAt.Unix(), Manifest: m}); err != nil && bg.cache.Path != "" {
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
	bg := &a.backgrounds
	if !backgrounds.ValidGameID(gameID) {
		return []string{}
	}
	local := func() []string {
		files := bg.store.List(gameID)
		urls := make([]string, 0, len(files))
		for _, f := range files {
			urls = append(urls, backgrounds.LocalURL(gameID, f.Name))
		}
		return urls
	}
	if a.GetPreferences().BackgroundSource == preferences.BackgroundSourceOffline {
		return local()
	}
	manifest, _ := a.backgroundManifest(false)
	if p, ok := manifest.Pack(gameID); ok && len(p.Files) > 0 {
		urls := make([]string, 0, len(p.Files))
		for _, f := range p.Files {
			urls = append(urls, bg.source.RawURL(bg.fetcher.Raw(), gameID, f.Name))
		}
		return urls
	}
	return local()
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
	d := backgrounds.Downloader{
		Store:     bg.store,
		URL:       func(gameID, name string) string { return bg.source.RawURL(bg.fetcher.Raw(), gameID, name) },
		UserAgent: bg.fetcher.UserAgent,
	}
	go func() {
		log := applog.For("Backgrounds")
		timer := log.Begin()
		res, err := d.Run(ctx, jobs, func(p backgrounds.Progress) { a.emit("background-download", p) })
		bg.dlMu.Lock()
		bg.dlCancel = nil
		bg.dlMu.Unlock()
		cancel()
		switch {
		case err != nil:
			timer.Warnf("background download stopped after %d image%s (%s)", res.Downloaded, plural(res.Downloaded), err)
		case res.Failed > 0:
			timer.Warnf("downloaded %d background image%s (%s), %d failed", res.Downloaded, plural(res.Downloaded), formatMB(res.Bytes), res.Failed)
		default:
			timer.Infof("downloaded %d background image%s (%s), %d already there", res.Downloaded, plural(res.Downloaded), formatMB(res.Bytes), res.Skipped)
		}
		a.emit("background-packs-changed")
	}()
	return nil
}

func formatMB(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

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
