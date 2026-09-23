package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pkg/browser"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Official-Husko/parallax-mod-manager/internal/about"
	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/checksum"
	"github.com/Official-Husko/parallax-mod-manager/internal/collection"
	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/dlc"
	"github.com/Official-Husko/parallax-mod-manager/internal/dlcstore"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/gamemedia"
	"github.com/Official-Husko/parallax-mod-manager/internal/launch"
	"github.com/Official-Husko/parallax-mod-manager/internal/launcherdb"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabtracking"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modnotes"
	"github.com/Official-Husko/parallax-mod-manager/internal/modpins"
	"github.com/Official-Husko/parallax-mod-manager/internal/modupdates"
	"github.com/Official-Husko/parallax-mod-manager/internal/patchoverride"
	"github.com/Official-Husko/parallax-mod-manager/internal/playset"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
	"github.com/Official-Husko/parallax-mod-manager/internal/resolvedconflicts"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
	"github.com/Official-Husko/parallax-mod-manager/internal/steam"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
	"github.com/Official-Husko/parallax-mod-manager/internal/versionignore"
	"github.com/Official-Husko/parallax-mod-manager/internal/watch"
)

// modWatchDebounce absorbs a burst of filesystem events from one logical
// change (a bulk copy, an archive extract) into a single refresh.
const modWatchDebounce = 400 * time.Millisecond

// modWatchMuteGrace is how long the mod folder watcher stays muted after the
// app finishes writing into that folder itself: the debounce, plus slack for the
// operating system to deliver the last events of the burst.
const modWatchMuteGrace = modWatchDebounce + time.Second

// App is the Wails-bound backend: a thin adapter that resolves real OS
// paths (the one place this project's "never resolve a real path inside a
// package that might be tested" rule doesn't apply, per CLAUDE.md) and
// delegates everything else to internal/ packages.
type App struct {
	ctx           context.Context
	registry      *game.Registry
	startupNotice string
	cacheDir      string
	playsets      playset.Store
	// collections persists user-defined, cross-game mod groupings for the
	// Library screen - see internal/collection. Distinct from playsets:
	// not per-game, not a load order.
	collections collection.Store
	gameMedia   gamemedia.Store
	preferences preferences.Preferences
	// preferencesMu guards every read and write of preferences: Wails
	// dispatches each frontend-triggered call on its own goroutine, and
	// preferences.Preferences now carries a map field (GamePaths) - an
	// unsynchronized concurrent map access in Go panics the whole process,
	// not just corrupts data, so this can't be left unguarded the way a
	// plain-value struct could have been.
	preferencesMu   sync.Mutex
	preferencesPath string
	// patchOverrides persists manual per-conflict winner overrides for
	// GeneratePatch - see internal/patchoverride and docs/patch-mods.md.
	// Dir is empty (methods degrade gracefully) when configDir couldn't
	// be resolved.
	patchOverrides patchoverride.Store
	// versionIgnore persists which mods a user has chosen to suppress the
	// version-incompatibility warning for - see internal/versionignore.
	// Dir is empty (methods degrade gracefully) when configDir couldn't
	// be resolved.
	versionIgnore versionignore.Store
	// modPins persists which mods are pinned to the top of the Editor's and the
	// Library's mod lists - see internal/modpins. Dir is empty (methods degrade
	// gracefully) when configDir couldn't be resolved.
	modPins modpins.Store
	// resolvedConflicts persists which contested conflict keys a user has
	// manually marked reviewed in the Conflict Resolver - a bookkeeping
	// flag only, see internal/resolvedconflicts. Dir is empty (methods
	// degrade gracefully) when configDir couldn't be resolved.
	resolvedConflicts resolvedconflicts.Store
	// modUpdates remembers what each game's mods looked like at the last startup
	// and reports what changed since - see internal/modupdates and
	// CheckModUpdates. Its Store is set in startup; zero-value usable.
	modUpdates modupdates.Tracker
	// backgrounds is the rotating background art's state: where it is published,
	// the offline copy on disk, the listing cache and the running download - see
	// backgrounds.go.
	backgrounds backgroundState
	// patchThumbnailPath is where a user-supplied patch_thumbnail.png would
	// live (empty when configDir couldn't be resolved) - see patchThumbnail.
	patchThumbnailPath string
	// configAppDir is this app's own folder under the user's config dir
	// (empty when configDir couldn't be resolved) - shown on the About page.
	configAppDir string
	// logDir is the folder the activity log file lives in (empty when file
	// logging couldn't start) - see initLogging.
	logDir     string
	steamRoots []string
	// checksumMu guards checksumCancel: the calculation running for each game, so a newer
	// request for the same game can stop the older one (see checksum.go).
	// modNotes keeps the person's own notes about individual mods, and modNotesMu
	// serialises changing them (each change reads the file, edits it and writes it).
	modNotes       modnotes.Store
	modNotesMu     sync.Mutex
	// loverslabInstalls tracks which local mods came from LoversLab, for the update
	// check - see internal/loverslabtracking and loverslabinstall.go.
	loverslabInstalls loverslabtracking.Store
	// installMu guards installCancel: the download running for each LoversLabInstall
	// request, so a Cancel call can reach and stop it - the same reason duplicateMu
	// guards duplicateCancel just below.
	installMu     sync.Mutex
	installCancel map[string]context.CancelFunc
	checksumMu     sync.Mutex
	checksumCancel map[string]context.CancelFunc
	// duplicateMu guards duplicateCancel: the copy running for each Duplicate request, so an
	// explicit CancelDuplicate call (see newmod.go) can reach and stop it. Unlike
	// checksumCancel, which only ever cancels an older run when a newer one starts, this one
	// is reached from a user-facing Cancel button while the copy is still running.
	duplicateMu     sync.Mutex
	duplicateCancel map[string]context.CancelFunc
	// checksumCache lets repeated PlaysetChecksum calls for the same game and mod set skip
	// reading and hashing file content that has not changed since the last call - see
	// checksum.ResultCache. The zero value is ready to use.
	checksumCache checksum.ResultCache
	modWatcher    *watch.FolderWatcher
	watchedGameID string
	// watchMute silences modWatcher while the app is itself writing into the
	// mod folder (generating the patch, purging mods): those operations
	// refresh the mod list on their own. Zero-value usable.
	watchMute watch.Mute
	// eventSink, when set, receives the events emit would send to the frontend -
	// how tests see them. nil in the real app.
	eventSink func(name string, data ...any)
	// gameProcMu guards gameExes and gameRunning: what is known about each game's
	// process (see gameplay.go). gameProcWarned remembers which games have had a
	// "can't list processes" warning, so it is said once.
	gameProcMu     sync.Mutex
	gameExes       map[string]cachedGameExe
	gameRunning    map[string]bool
	gameProcWarned sync.Map
	// gameLog is the game log file currently being followed (WatchGameLog), and
	// gameLogLast the newest session number handed out, so a session number is
	// never reused. Guarded by gameLogMu.
	gameLogMu   sync.Mutex
	gameLog     *gameLogFollow
	gameLogLast int
	// patchLogged remembers the last patch status line written per game, so a
	// state that hasn't changed isn't repeated on every scan. Zero-value usable.
	patchLogged sync.Map
	// workshopDetails holds real Steam Workshop metadata in memory for the
	// app's runtime - see library.WorkshopDetailsCache. Zero-value usable.
	workshopDetails library.WorkshopDetailsCache
	// workshopPages remembers which Workshop item pages were up when looked at, for
	// telling a deleted item from an unlisted one - see confirmWorkshopPages.
	workshopPages pageLiveCache
	// backup is mod preservation: copies of Workshop mods that are deleted or private,
	// made before Steam removes their files - see backups.go.
	backup backupState
	// steam is the optional Steam Web API key and the service that decides which
	// Steam API answers Workshop details - see steamapi_settings.go. Set up by
	// initSteamAPI in startup.
	steam steamAPIState
	// loverslab is the optional saved LoversLab sign-in for the Browsing Extensions
	// page - see loverslab_settings.go. Set up by initLoversLab in startup.
	loverslab loversLabState
	// authorProfiles holds real Steam Community profiles for Workshop mod
	// authors in memory for the app's runtime - see
	// library.AuthorProfileCache. Zero-value usable.
	authorProfiles library.AuthorProfileCache
	// changelogs holds real Steam Workshop update notes in memory for the
	// app's runtime, fetched per mod on demand - see
	// library.ChangelogCache. Zero-value usable.
	changelogs library.ChangelogCache
	// contentPaths remembers each scanned mod's content directory from the
	// last ScanGame, so ReadModFile (called once per side each time the
	// Conflict Resolver switches contested keys) can skip re-scanning the
	// whole game - see library.ContentPathCache. Zero-value usable.
	contentPaths library.ContentPathCache
	// dlcRefreshing tracks which games already have a DLC Store-data
	// background refresh in flight, so a burst of DLCStoreData calls
	// (e.g. the DLC screen re-rendering) never kicks off more than one at
	// once for the same game.
	dlcRefreshingMu sync.Mutex
	dlcRefreshing   map[string]bool
}

// NewApp creates a new App application struct. Real setup (resolving OS
// paths, loading the games list) happens in startup, once a context
// exists - see that method.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so we can
// call the runtime methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.steamRoots = steam.DefaultRoots()

	configDir, configErr := os.UserConfigDir()

	gamesListPath := ""
	if configErr == nil {
		gamesListPath = filepath.Join(configDir, "parallax-mod-manager", "games.jsonc")
	}
	registry, notice, err := game.LoadRegistry(AppVersion, embeddedGamesList, gamesListPath)
	if err != nil {
		// The embedded default itself failed to parse - a build-time bug,
		// not something a running app can recover from.
		panic(fmt.Sprintf("app: embedded games list is invalid: %v", err))
	}
	a.registry = registry
	a.startupNotice = notice

	if dir, err := os.UserCacheDir(); err == nil {
		a.cacheDir = filepath.Join(dir, "parallax-mod-manager")
	}
	a.initLogging(ctx)
	build := about.Collect(AppName, AppVersion)
	applog.For("App").Infof("%s %s starting (go %s, %s/%s, %d CPUs)", AppName, AppVersion, build.GoVersion, build.OS, build.Arch, runtime.NumCPU())
	// What the rest of the log is about: the build and the computer, pinned to the top.
	a.logSystemReport(build)
	if notice != "" {
		applog.For("Games").Warnf("%s", notice)
	}
	applog.For("Games").Infof("games list loaded: %d games", len(registry.List()))
	if configErr == nil {
		a.playsets = playset.FileStore{Dir: filepath.Join(configDir, "parallax-mod-manager", "playsets")}
		a.collections = collection.FileStore{Dir: filepath.Join(configDir, "parallax-mod-manager", "collections")}
		a.patchOverrides = patchoverride.Store{Dir: filepath.Join(configDir, "parallax-mod-manager", "patch_overrides")}
		a.modNotes = modnotes.Store{Dir: filepath.Join(configDir, "parallax-mod-manager", "mod_notes")}
		a.loverslabInstalls = loverslabtracking.Store{Dir: filepath.Join(configDir, "parallax-mod-manager", "loverslab_installs")}
		a.versionIgnore = versionignore.Store{Dir: filepath.Join(configDir, "parallax-mod-manager", "version_ignore")}
		a.modPins = modpins.Store{Dir: filepath.Join(configDir, "parallax-mod-manager", "mod_pins")}
		a.resolvedConflicts = resolvedconflicts.Store{Dir: filepath.Join(configDir, "parallax-mod-manager", "resolved_conflicts")}
		a.modUpdates.Store = modupdates.Store{Dir: modUpdatesDir(filepath.Join(configDir, "parallax-mod-manager"))}
		a.initBackgrounds(filepath.Join(configDir, "parallax-mod-manager"))
		a.patchThumbnailPath = filepath.Join(configDir, "parallax-mod-manager", "patch_thumbnail.png")
		a.configAppDir = filepath.Join(configDir, "parallax-mod-manager")
	}
	a.initSteamAPI(a.configAppDir)
	a.initLoversLab(a.configAppDir)
	a.initBackups(a.configAppDir)

	mediaFS, err := fs.Sub(embeddedGameMedia, "data/game_media")
	if err == nil {
		a.gameMedia.Embedded = mediaFS
	}
	if configErr == nil {
		a.gameMedia.OverrideDir = filepath.Join(configDir, "parallax-mod-manager", "game_media")
	}

	if configErr == nil {
		a.preferencesPath = filepath.Join(configDir, "parallax-mod-manager", "preferences.jsonc")
		a.preferences = preferences.Load(a.preferencesPath)
	} else {
		a.preferences = preferences.Defaults()
	}

	// The app's own surroundings and settings, once they are all loaded.
	go a.logEnvironmentReport()
}

// shutdown is called when the app is closing, before the runtime exits.
func (a *App) shutdown(ctx context.Context) {
	a.modWatcher.Close()
	a.StopWatchingGameLog()
	a.stopBackups()
	applog.For("App").Infof("shutting down")
	applog.Default().Close()
}

// initLogging points the activity log (internal/applog) at a rotating file in
// the cache folder and starts streaming new lines to the frontend as "log-batch"
// events. Lines are handed over in small batches on a timer rather than one
// event each, so a burst (a scan logging a dozen steps) is one message across
// the JS boundary, not a dozen. If the file can't be opened the log still works
// in memory and in the UI - it just isn't kept on disk.
func (a *App) initLogging(ctx context.Context) {
	hub := applog.Default()
	if a.cacheDir != "" {
		if err := hub.SetFile(filepath.Join(a.cacheDir, "logs", "app.log"), 2<<20); err != nil {
			applog.For("App").Warnf("the log file couldn't be opened, so this session's log is memory-only: %v", err)
		}
	}
	if path := hub.FilePath(); path != "" {
		a.logDir = filepath.Dir(path)
	}

	lines := make(chan applog.Entry, 4096)
	cancel := hub.Subscribe(func(e applog.Entry) {
		select {
		case lines <- e:
		default:
			// The UI can't keep up: drop the live copy. The line is still in
			// the ring and the file, and the frontend re-reads the ring
			// whenever it opens the log view.
		}
	})
	go func() {
		defer cancel()
		tick := time.NewTicker(150 * time.Millisecond)
		defer tick.Stop()
		var batch []applog.Entry
		for {
			select {
			case e := <-lines:
				batch = append(batch, e)
			case <-tick.C:
				if len(batch) > 0 {
					wailsruntime.EventsEmit(ctx, "log-batch", batch)
					batch = nil
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// LogEntries returns the activity log lines currently in memory, oldest first,
// for the About page's log view to start from before it follows live lines.
func (a *App) LogEntries() []applog.Entry {
	return applog.Default().Entries()
}

// ClearLog empties the in-memory log the About page shows. The log file on disk
// is kept.
func (a *App) ClearLog() {
	applog.Default().Clear()
}

// LogEvent lets the frontend add a line to the activity log - for things that
// only the UI knows happened (an autosort run, a prompt being shown). level is
// "debug", "info", "warn" or "error" (anything else is info); component is the
// tag shown in brackets.
func (a *App) LogEvent(level, component, message string) {
	applog.Default().Log(applog.ParseLevel(level), component, message, false, 0)
}

// gameLabel is gameID's display name for log lines, or the ID itself when it
// isn't a registered game.
func (a *App) gameLabel(gameID string) string {
	if cfg, ok := a.registry.Get(gameID); ok {
		return cfg.DisplayName
	}
	return gameID
}

// StartupNotice returns a user-facing message when something noteworthy
// happened while loading app data at startup (currently: a custom
// games.jsonc override was rejected and the built-in list is active
// instead) - empty when there's nothing to say. The frontend calls this
// once and shows it as a dismissible banner.
func (a *App) StartupNotice() string {
	return a.startupNotice
}

// GameMedia returns kind ("logo" or "background") art for gameID as a
// data: URI, or "" (no error) if none exists yet - the frontend shows its
// plain color-swatch fallback in that case.
func (a *App) GameMedia(kind, gameID string) (string, error) {
	data, mimeType, ok := a.gameMedia.Find(kind, gameID)
	if !ok {
		return "", nil
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// clonePreferences returns p with its GamePaths map and ManagedGames slice
// copied rather than shared - both GetPreferences (handing a value to a
// caller who then does its own read-modify-write) and SetPreferences
// (adopting a caller-supplied value as the new source of truth) need this
// so two goroutines never end up holding, and concurrently mutating, the
// same underlying map.
func clonePreferences(p preferences.Preferences) preferences.Preferences {
	if p.ManagedGames != nil {
		p.ManagedGames = append([]string(nil), p.ManagedGames...)
	}
	if p.GamePaths != nil {
		paths := make(map[string]string, len(p.GamePaths))
		for k, v := range p.GamePaths {
			paths[k] = v
		}
		p.GamePaths = paths
	}
	if p.ExtraModFolders != nil {
		folders := make(map[string][]string, len(p.ExtraModFolders))
		for k, v := range p.ExtraModFolders {
			folders[k] = append([]string(nil), v...)
		}
		p.ExtraModFolders = folders
	}
	if p.BackgroundStaticImages != nil {
		images := make(map[string]string, len(p.BackgroundStaticImages))
		for k, v := range p.BackgroundStaticImages {
			images[k] = v
		}
		p.BackgroundStaticImages = images
	}
	if p.LastSeenGameVersions != nil {
		versions := make(map[string]string, len(p.LastSeenGameVersions))
		for k, v := range p.LastSeenGameVersions {
			versions[k] = v
		}
		p.LastSeenGameVersions = versions
	}
	return p
}

// GetPreferences returns the app's current preferences.
func (a *App) GetPreferences() preferences.Preferences {
	a.preferencesMu.Lock()
	defer a.preferencesMu.Unlock()
	return clonePreferences(a.preferences)
}

// SetPreferences persists p and applies it immediately: if scanning for
// new mods was just turned on or off, the live watcher for whichever game
// is currently being watched (see WatchMods) is started or stopped right
// away rather than waiting for the next game switch.
//
// Whatever p carries for LastSeenGameVersions and BackgroundStaticImages is ignored:
// those fields are the backend's own records (see CheckGameUpdates and
// SetStaticBackground), and the frontend's copy of the preferences may be older than
// them.
func (a *App) SetPreferences(p preferences.Preferences) error {
	return a.savePreferences(p, true)
}

// savePreferences is SetPreferences' body. keepBackendOwned says whether to
// hold on to the current values of the fields only the backend changes; code
// that changes one of them itself passes false.
func (a *App) savePreferences(p preferences.Preferences, keepBackendOwned bool) error {
	a.preferencesMu.Lock()
	p.BackgroundSource = preferences.NormalizedBackgroundSource(p.BackgroundSource)
	p.BackgroundBlur = preferences.NormalizedPercent(p.BackgroundBlur)
	p.BackgroundDarken = preferences.NormalizedPercent(p.BackgroundDarken)
	p.AccentMode = preferences.NormalizedAccentMode(p.AccentMode)
	p.AccentColor = preferences.NormalizedHexColor(p.AccentColor)
	if keepBackendOwned {
		p.LastSeenGameVersions = a.preferences.LastSeenGameVersions
		p.BackgroundStaticImages = a.preferences.BackgroundStaticImages
	}
	if a.preferencesPath != "" {
		if err := preferences.Save(a.preferencesPath, p); err != nil {
			a.preferencesMu.Unlock()
			return err
		}
	}
	changed := preferences.ChangedKeys(a.preferences, p)
	watchingChanged := a.preferences.ScanForNewMods != p.ScanForNewMods
	a.preferences = clonePreferences(p)
	a.preferencesMu.Unlock()
	if len(changed) > 0 {
		applog.For("Settings").Infof("preferences saved (%s)", strings.Join(changed, ", "))
	}

	// Only turning "scan for new mods" on or off changes what the watcher
	// should be doing - every other save (the last playset, the selected
	// game...) would otherwise tear down and rebuild an identical watch.
	if watchingChanged && a.watchedGameID != "" {
		return a.WatchMods(a.watchedGameID)
	}
	return nil
}

// WatchMods starts watching gameID's mod folder for changes, replacing any
// previous watch. The frontend calls this whenever the active game
// changes so its mod list can update live. Watching is skipped (any
// existing watcher is still stopped first) when the "scan for new mods"
// preference is off, or when the mod folder doesn't exist yet (nothing to
// watch, not an error).
func (a *App) WatchMods(gameID string) error {
	a.modWatcher.Close()
	a.modWatcher = nil
	a.watchedGameID = gameID

	if !a.preferences.ScanForNewMods {
		return nil
	}

	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	userDir, err := cfg.UserDataDir()
	if err != nil {
		return nil
	}
	modDir := filepath.Join(userDir, "mod")
	if _, err := os.Stat(modDir); err != nil {
		return nil
	}

	w, err := watch.New(modDir, modWatchDebounce, func() {
		if a.watchMute.Muted() {
			applog.For("Watch").Debugf("'%s' mod folder changed by the app itself - not refreshing again", cfg.DisplayName)
			return
		}
		applog.For("Watch").Infof("'%s' mod folder changed - refreshing", cfg.DisplayName)
		wailsruntime.EventsEmit(a.ctx, "mods-changed", gameID)
	}, func(err error) {
		applog.For("Watch").Warnf("watching '%s' reported: %v", modDir, err)
	})
	if err != nil {
		applog.For("Watch").Errorf("couldn't watch '%s': %v", modDir, err)
		return err
	}
	a.modWatcher = w
	applog.For("Watch").Infof("watching '%s' for changes", modDir)
	return nil
}

// ListGames returns every game this build supports, for a game picker.
func (a *App) ListGames() []library.GameInfo {
	games := a.registry.List()
	infos := make([]library.GameInfo, len(games))
	for i, g := range games {
		infos[i] = library.GameInfo{ID: g.ID, DisplayName: g.DisplayName}
	}
	return infos
}

// DetectGames reports every registered game's real install and mod-folder
// state, for the first-run wizard's "games found" step and the Manage
// Games / Paths & Folders settings screens.
func (a *App) DetectGames() ([]library.DetectedGame, error) {
	games := a.registry.List()
	result := make([]library.DetectedGame, 0, len(games))
	for _, cfg := range games {
		d, err := a.detectGameConsideringOverride(cfg)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	installed := 0
	for _, d := range result {
		if d.Installed {
			installed++
		}
	}
	applog.For("Games").Infof("detected %d of %d supported games installed", installed, len(result))
	return result, nil
}

// gamePathOverride returns cfg's manually-set install path override, if one
// is persisted and still verifies against cfg's signature files. A stale
// override (the folder was moved, removed, or the game was reinstalled
// elsewhere) is treated as if it were never set, falling back to automatic
// detection rather than trusting a path that no longer checks out.
func (a *App) gamePathOverride(cfg game.GameConfig) (string, bool) {
	a.preferencesMu.Lock()
	dir, ok := a.preferences.GamePaths[cfg.ID]
	a.preferencesMu.Unlock()
	if !ok || dir == "" || !cfg.VerifyInstallDir(dir) {
		return "", false
	}
	return dir, true
}

// extraModFolders returns gameID's configured extra mod folders (see
// preferences.Preferences.ExtraModFolders), safe to pass straight into
// scan.Options/library.Options.ExtraFolders.
func (a *App) extraModFolders(gameID string) []string {
	a.preferencesMu.Lock()
	defer a.preferencesMu.Unlock()
	return append([]string(nil), a.preferences.ExtraModFolders[gameID]...)
}

// setGamePathOverride persists dir as cfg's manually-chosen install path -
// see preferences.Preferences.GamePaths.
func (a *App) setGamePathOverride(gameID, dir string) error {
	a.preferencesMu.Lock()
	defer a.preferencesMu.Unlock()
	if a.preferences.GamePaths == nil {
		a.preferences.GamePaths = map[string]string{}
	}
	a.preferences.GamePaths[gameID] = dir
	if a.preferencesPath == "" {
		return nil
	}
	return preferences.Save(a.preferencesPath, a.preferences)
}

// detectGameConsideringOverride is DetectGames' per-game logic: a
// still-valid manual path override always wins over automatic detection,
// since it reflects an explicit user choice.
func (a *App) detectGameConsideringOverride(cfg game.GameConfig) (library.DetectedGame, error) {
	extra := a.extraModFolders(cfg.ID)
	if override, ok := a.gamePathOverride(cfg); ok {
		d, err := library.DetectGameAt(a.ctx, cfg, override, a.steamRoots, extra)
		if err != nil {
			return library.DetectedGame{}, err
		}
		d.PathOverridden = true
		return d, nil
	}
	return library.DetectGame(a.ctx, cfg, a.steamRoots, extra)
}

// resolveInstallDir returns cfg's real install directory - a still-valid
// manual override if one is set, else automatic Steam-library detection -
// the same precedence detectGameConsideringOverride uses, without paying
// for that method's full mod scan when only the directory itself is
// needed (LaunchGame's direct-launch path, below). ok is false when
// neither resolves to a real install.
func (a *App) resolveInstallDir(cfg game.GameConfig) (string, bool) {
	if override, ok := a.gamePathOverride(cfg); ok {
		return override, true
	}
	return cfg.DetectInstall()
}

// launchOptionsFor builds LaunchGame's launch.LaunchOptions from cfg's
// per-game launch mode preference (preferences.Preferences.LaunchModes).
// launch.LaunchModeSteam - including an unset preference - leaves opts
// zero-valued, matching Launch's own Steam-first default. LaunchModeDirect
// resolves cfg's real install directory so Launch can skip the Paradox
// Launcher entirely, per docs/game-launching.md; that resolution can fail
// (a game set to direct mode that was never installed, or whose install
// moved), which is reported back as an error rather than silently falling
// through to the Steam path the user explicitly opted out of.
func (a *App) launchOptionsFor(cfg game.GameConfig) (launch.LaunchOptions, error) {
	a.preferencesMu.Lock()
	mode := a.preferences.LaunchModes[cfg.ID]
	a.preferencesMu.Unlock()

	if mode != string(launch.LaunchModeDirect) {
		return launch.LaunchOptions{}, nil
	}

	installDir, ok := a.resolveInstallDir(cfg)
	if !ok {
		return launch.LaunchOptions{}, fmt.Errorf("app: %s is set to launch directly, but its install directory couldn't be found - set its path under Paths & folders", cfg.DisplayName)
	}
	return launch.LaunchOptions{Direct: true, InstallDir: installDir}, nil
}

// GameVersion returns gameID's real, currently-installed version (e.g.
// "v4.4.6"), for the TopBar's game pill and the Workspace's own mod
// version-compatibility flagging. "" (never an error) when the game isn't
// installed, or its launcher-settings.json doesn't carry one - see
// game.GameConfig.GameVersion.
func (a *App) GameVersion(gameID string) (string, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return "", fmt.Errorf("app: unknown game %q", gameID)
	}
	if override, ok := a.gamePathOverride(cfg); ok {
		return cfg.GameVersion(override), nil
	}
	installDir, installed := cfg.DetectInstall()
	if !installed {
		return "", nil
	}
	return cfg.GameVersion(installDir), nil
}

// BrowseForGameInstall opens a native folder-picker so a user can point at
// a game install automatic Steam-library detection didn't find (a non-Steam
// copy, an unusual library setup). Returns the game's current detected
// state unchanged if the user cancels the dialog (an empty path is not an
// error); returns an error if a folder was chosen but doesn't actually
// contain the game (verified against SignatureFiles) - never trusts an
// unverified folder just because the user picked it. A verified pick is
// persisted as a path override (preferences.Preferences.GamePaths) so it
// survives past this session.
func (a *App) BrowseForGameInstall(gameID string) (library.DetectedGame, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.DetectedGame{}, fmt.Errorf("app: unknown game %q", gameID)
	}

	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: fmt.Sprintf("Select the %s install folder", cfg.DisplayName),
	})
	if err != nil {
		return library.DetectedGame{}, err
	}
	if dir == "" {
		return a.detectGameConsideringOverride(cfg)
	}
	if !cfg.VerifyInstallDir(dir) {
		applog.For("Games").Warnf("'%s' doesn't look like a '%s' install, so it was not used", dir, cfg.DisplayName)
		return library.DetectedGame{}, fmt.Errorf("app: %s does not look like a %s install", dir, cfg.DisplayName)
	}
	if err := a.setGamePathOverride(gameID, dir); err != nil {
		applog.For("Games").Errorf("couldn't save the install folder for '%s': %v", cfg.DisplayName, err)
		return library.DetectedGame{}, err
	}
	applog.For("Games").Infof("install folder for '%s' set to '%s'", cfg.DisplayName, dir)
	d, err := library.DetectGameAt(a.ctx, cfg, dir, a.steamRoots, a.extraModFolders(gameID))
	if err != nil {
		return library.DetectedGame{}, err
	}
	d.PathOverridden = true
	return d, nil
}

// BrowseForAnyGameInstall opens the same folder-picker as
// BrowseForGameInstall, but without a specific game in mind: the chosen
// folder is checked against every registered game, and whichever one it
// verifies against is returned. Useful when a game wasn't auto-detected and
// the user isn't sure (or doesn't want to hunt for) which row to browse
// from. Returns a zero-value DetectedGame (empty ID) if the user cancels;
// errors only if a folder was chosen but matches no registered game. A
// match is persisted as a path override, same as BrowseForGameInstall.
func (a *App) BrowseForAnyGameInstall() (library.DetectedGame, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select a game install folder",
	})
	if err != nil {
		return library.DetectedGame{}, err
	}
	if dir == "" {
		return library.DetectedGame{}, nil
	}
	for _, cfg := range a.registry.List() {
		if !cfg.VerifyInstallDir(dir) {
			continue
		}
		if err := a.setGamePathOverride(cfg.ID, dir); err != nil {
			return library.DetectedGame{}, err
		}
		d, err := library.DetectGameAt(a.ctx, cfg, dir, a.steamRoots, a.extraModFolders(cfg.ID))
		if err != nil {
			return library.DetectedGame{}, err
		}
		d.PathOverridden = true
		return d, nil
	}
	return library.DetectedGame{}, fmt.Errorf("app: %s does not match any registered game", dir)
}

// ClearGamePath removes any manually-set install path override for gameID,
// reverting to automatic Steam-library detection, and reports the game's
// state after doing so.
func (a *App) ClearGamePath(gameID string) (library.DetectedGame, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.DetectedGame{}, fmt.Errorf("app: unknown game %q", gameID)
	}

	a.preferencesMu.Lock()
	if a.preferences.GamePaths != nil {
		delete(a.preferences.GamePaths, gameID)
		if a.preferencesPath != "" {
			if err := preferences.Save(a.preferencesPath, a.preferences); err != nil {
				a.preferencesMu.Unlock()
				return library.DetectedGame{}, err
			}
		}
	}
	a.preferencesMu.Unlock()
	applog.For("Games").Infof("install folder override for '%s' cleared - using automatic detection", cfg.DisplayName)

	return library.DetectGame(a.ctx, cfg, a.steamRoots, a.extraModFolders(gameID))
}

// SetGameManaged adds or removes gameID from the set of games Parallax Mod
// Manager actively manages - only managed games show up in the game
// switcher, Library, DLC, and Workspace. See
// preferences.Preferences.ManagedGames.
func (a *App) SetGameManaged(gameID string, managed bool) error {
	if _, ok := a.registry.Get(gameID); !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}

	a.preferencesMu.Lock()
	set := make(map[string]bool, len(a.preferences.ManagedGames))
	for _, id := range a.preferences.ManagedGames {
		set[id] = true
	}
	if managed {
		set[gameID] = true
	} else {
		delete(set, gameID)
	}
	// Rebuilt in registry order (rather than map iteration order) so the
	// persisted list stays stable and diff-friendly across saves.
	ids := make([]string, 0, len(set))
	for _, cfg := range a.registry.List() {
		if set[cfg.ID] {
			ids = append(ids, cfg.ID)
		}
	}
	next := a.preferences
	next.ManagedGames = ids
	a.preferencesMu.Unlock()

	if managed {
		applog.For("Games").Infof("now managing '%s'", a.gameLabel(gameID))
	} else {
		applog.For("Games").Infof("stopped managing '%s'", a.gameLabel(gameID))
	}
	return a.SetPreferences(next)
}

// BrowseForExtraModFolder opens a native folder-picker so a user can add an
// extra folder to search recursively for gameID's mods, beyond its own
// managed mod folder - see preferences.Preferences.ExtraModFolders and
// scan.ScanExtraFolder. Returns "" (no error) if the user cancels the
// dialog, or if the chosen folder is already in gameID's list.
func (a *App) BrowseForExtraModFolder(gameID string) (string, error) {
	if _, ok := a.registry.Get(gameID); !ok {
		return "", fmt.Errorf("app: unknown game %q", gameID)
	}

	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select an extra folder to search for mods",
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil
	}

	a.preferencesMu.Lock()
	existing := a.preferences.ExtraModFolders[gameID]
	for _, e := range existing {
		if e == dir {
			a.preferencesMu.Unlock()
			return "", nil
		}
	}
	next := a.preferences
	folders := make(map[string][]string, len(next.ExtraModFolders)+1)
	for k, v := range next.ExtraModFolders {
		folders[k] = v
	}
	folders[gameID] = append(append([]string(nil), existing...), dir)
	next.ExtraModFolders = folders
	a.preferencesMu.Unlock()

	if err := a.SetPreferences(next); err != nil {
		applog.For("Games").Errorf("couldn't add the extra mod folder '%s': %v", dir, err)
		return "", err
	}
	applog.For("Games").Infof("added extra mod folder '%s' for '%s'", dir, a.gameLabel(gameID))
	return dir, nil
}

// RemoveExtraModFolder removes path from gameID's extra mod folders.
func (a *App) RemoveExtraModFolder(gameID, path string) error {
	a.preferencesMu.Lock()
	existing := a.preferences.ExtraModFolders[gameID]
	kept := make([]string, 0, len(existing))
	for _, e := range existing {
		if e != path {
			kept = append(kept, e)
		}
	}
	next := a.preferences
	folders := make(map[string][]string, len(next.ExtraModFolders))
	for k, v := range next.ExtraModFolders {
		folders[k] = v
	}
	if len(kept) == 0 {
		delete(folders, gameID)
	} else {
		folders[gameID] = kept
	}
	next.ExtraModFolders = folders
	a.preferencesMu.Unlock()

	applog.For("Games").Infof("removed extra mod folder '%s' for '%s'", path, a.gameLabel(gameID))
	return a.SetPreferences(next)
}

// OpenPath opens an arbitrary filesystem path in the OS file manager - used
// by the Paths & Folders settings screen for a game's install directory or
// mod folder. See OpenModFolder for the equivalent scoped to one mod.
func (a *App) OpenPath(path string) error {
	if path == "" {
		return fmt.Errorf("app: no path to open")
	}
	if err := browser.OpenFile(path); err != nil {
		applog.For("Files").Warnf("couldn't open '%s': %v", path, err)
		return err
	}
	return nil
}

// ScanGame scans, parses, and resolves conflicts for one supported game.
// playsetName is optional; empty means no selection made yet - every
// scanned mod starts disabled.
func (a *App) ScanGame(gameID, playsetName string) (library.Summary, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.Summary{}, fmt.Errorf("app: unknown game %q", gameID)
	}

	log := applog.For("Scan")
	timer := log.Begin()
	// Known only when the game is installed; the patch check treats an
	// unknown version as "can't tell", not as a change.
	gameVersion, _ := a.GameVersion(gameID)
	if playsetName != "" {
		log.Infof("scanning '%s' with playset '%s'", cfg.DisplayName, playsetName)
	} else {
		log.Infof("scanning '%s' (no playset selected)", cfg.DisplayName)
	}

	opts := library.Options{
		CacheDir:     a.cacheDir,
		SteamRoots:   a.steamRoots,
		ExtraFolders: a.extraModFolders(gameID),
		Overrides:    a.patchOverrides.Load(gameID),
		ContentPaths: &a.contentPaths,
		GameVersion:  gameVersion,
		// The mod list itself (names/versions/sources) is known the moment
		// scanning finishes, well before conflict detection's slower
		// per-mod content parsing completes - emit it immediately so the
		// frontend can show the list right away instead of blocking on a
		// full "Scanning..." page.
		OnQuickSummary: func(s library.Summary) {
			timer.Infof("mod list ready: %d mods found", len(s.Mods))
			wailsruntime.EventsEmit(a.ctx, "scan-quick", gameID, s)
		},
	}
	if playsetName != "" {
		p, err := a.playsets.Load(a.ctx, gameID, playsetName)
		if err != nil {
			timer.Errorf("couldn't load playset '%s': %v", playsetName, err)
			return library.Summary{}, err
		}
		opts.Order = conflict.LoadOrder(p.ModIDs)
	}
	summary, err := library.LoadGame(a.ctx, cfg, opts)
	if err != nil {
		timer.Errorf("scan of '%s' failed: %v", cfg.DisplayName, err)
		return summary, err
	}
	enabled := 0
	for _, m := range summary.Mods {
		if m.Enabled {
			enabled++
		}
	}
	timer.Infof("'%s': %d mods (%d enabled), %d conflicts, %d issues", cfg.DisplayName, len(summary.Mods), enabled, len(summary.Conflicts), len(summary.Errors))
	// A few issues are worth a line each; the rest would only bury them.
	for i, e := range summary.Errors {
		if i == 5 {
			log.Warnf("... and %d more scan issues", len(summary.Errors)-5)
			break
		}
		log.Warnf("%s", e)
	}
	// With no mods loaded there is nothing to judge the patch against.
	if enabled > 0 {
		a.logPatchState(gameID, summary.Patch)
	}
	return summary, nil
}

// logPatchState writes gameID's generated-patch status to the log - but only
// when it differs from the last line written for that game. Every scan
// recomputes it, and the same "out of date" warning repeated on each one is
// noise that buries everything else.
func (a *App) logPatchState(gameID string, patch library.PatchSummary) {
	if !patch.Exists {
		a.patchLogged.Delete(gameID)
		return
	}
	log := applog.For("Patch")
	var level applog.Level
	var line string
	if patch.NeedsAttention() {
		level = applog.Warn
		line = fmt.Sprintf("the generated patch is out of date: %d changed, %d new, %d no longer conflicting%s", patch.Changed, patch.New, patch.Obsolete, patchVersionNote(patch))
	} else {
		level = applog.Info
		line = fmt.Sprintf("the generated patch (generation %d) is up to date: %d keys covered", patch.Generation, patch.Patched)
	}
	if prev, ok := a.patchLogged.Load(gameID); ok && prev == line {
		return
	}
	a.patchLogged.Store(gameID, line)
	if level == applog.Warn {
		log.Warnf("%s", line)
	} else {
		log.Infof("%s", line)
	}
}

// patchVersionNote is a log-line suffix saying the game has moved on since the
// patch was made, or "" when it hasn't.
func patchVersionNote(p library.PatchSummary) string {
	if !p.GameChanged {
		return ""
	}
	return fmt.Sprintf("; made for '%s', the game is now '%s'", p.GeneratedForVersion, p.GameVersion)
}

// ListModFiles returns modID's real on-disk file tree, for the Workspace
// detail panel's Files tab.
func (a *App) ListModFiles(gameID, modID string) (library.ModFiles, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.ModFiles{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	files, err := library.ListModFiles(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)}, modID)
	if err != nil {
		applog.For("Library").Warnf("couldn't list the files of mod '%s' in '%s': %v", modID, cfg.DisplayName, err)
	}
	return files, err
}

// ModThumbnail returns modID's real thumbnail image, if it has a usable
// one, as a data: URI - the same convention as GameMedia. Empty string, no
// error, means the mod has no thumbnail. See library.ModThumbnail.
func (a *App) ModThumbnail(gameID, modID string) (string, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return "", fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.ModThumbnail(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)}, modID)
}

// ModSizes returns every one of gameID's scanned mods' real on-disk content
// size (mod ID -> bytes), for the Library table's Size column.
func (a *App) ModSizes(gameID string) (map[string]int64, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	sizes, err := library.ModSizes(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		applog.For("Library").Warnf("couldn't measure the mods of '%s': %v", cfg.DisplayName, err)
	}
	return sizes, err
}

// ReadModFile returns one real file's text content from inside modID's
// content directory, for the conflict resolver's side-by-side view.
func (a *App) ReadModFile(gameID, modID, relPath string) (library.ModFileContent, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.ModFileContent{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	content, err := library.ReadModFile(a.ctx, cfg, library.Options{
		SteamRoots:   a.steamRoots,
		ExtraFolders: a.extraModFolders(gameID),
		ContentPaths: &a.contentPaths,
	}, modID, relPath)
	if err != nil {
		applog.For("Files").Warnf("couldn't read '%s' from mod '%s': %v", relPath, modID, err)
	}
	return content, err
}

// FindEmptyMods lists gameID's real local mods with no usable content, for
// the Workspace's "Purge empty" review dialog. See library.FindEmptyMods.
func (a *App) FindEmptyMods(gameID string) ([]library.EmptyModCandidate, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.FindEmptyMods(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
}

// PurgeMods deletes the descriptor file for each of modIDs, after the user
// has reviewed and confirmed them in the "Purge empty" dialog. See
// library.PurgeMods.
func (a *App) PurgeMods(gameID string, modIDs []string) (library.PurgeResult, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.PurgeResult{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	// The frontend refreshes the mod list itself once this returns.
	defer a.watchMute.Begin(modWatchMuteGrace)()
	result, err := library.PurgeMods(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)}, modIDs)
	if err != nil {
		applog.For("Library").Errorf("purging empty mods in '%s' failed: %v", cfg.DisplayName, err)
	} else {
		applog.For("Library").Infof("purged %d empty mods from '%s' (%d couldn't be deleted)", len(result.Deleted), cfg.DisplayName, len(result.Errors))
	}
	return result, err
}

// GeneratePatch resolves every genuine conflict in gameID's current mod
// set (order, exactly as the caller's own in-memory load order - not a
// saved playset, since that's what the Conflict Resolver the user is
// looking at was actually computed from) and writes a real patch mod
// pinning down each one's winner - a manual override from
// SetPatchOverride, if the user set one for that conflict, otherwise the
// automatic load-order winner. See library.GeneratePatch.
func (a *App) GeneratePatch(gameID string, order []string) (library.PatchResult, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.PatchResult{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	// The frontend rescans on its own when this returns, so the watcher
	// noticing the patch appear in the mod folder would only repeat that scan.
	defer a.watchMute.Begin(modWatchMuteGrace)()
	log := applog.For("Patch")
	timer := log.Begin()
	log.Infof("generating a patch for '%s' from %d loaded mods", cfg.DisplayName, len(order))
	// An unknown version isn't fatal: the patch just declares itself
	// compatible with any game version instead of a specific one.
	gameVersion, _ := a.GameVersion(gameID)
	result, err := library.GeneratePatch(a.ctx, cfg, library.Options{
		SteamRoots:     a.steamRoots,
		ExtraFolders:   a.extraModFolders(gameID),
		Order:          conflict.LoadOrder(order),
		Overrides:      a.patchOverrides.Load(gameID),
		GameVersion:    gameVersion,
		PatchThumbnail: a.patchThumbnail(),
	})
	switch {
	case err != nil:
		timer.Errorf("patch generation failed: %v", err)
	case !result.Written:
		timer.Infof("nothing to patch: %d conflicts skipped, none patchable", result.SkippedKeys)
	default:
		timer.Infof("generation %d written: %d keys patched, %d skipped", result.Generation, result.PatchedKeys, result.SkippedKeys)
		if result.SkippedKeys > 0 {
			log.Warnf("%d conflicts couldn't be written into the patch", result.SkippedKeys)
		}
	}
	return result, err
}

// AboutInfo returns everything the About page shows: this build's real
// version, commit, toolchain and platform, where this app keeps its settings,
// cache and log, how many games are registered, and the project's own links
// and credit. No network access.
func (a *App) AboutInfo() about.Info {
	info := about.Collect(AppName, AppVersion)
	info.ConfigDir = a.configAppDir
	info.CacheDir = a.cacheDir
	info.LogDir = a.logDir
	info.Games = len(a.registry.List())
	info.LicenceName, info.LicenceID = about.ParseLicence(embeddedLicence)
	return info
}

// LicenceText returns the full text of the project's licence (markdown), the one
// shipped inside this build - what the About page's licence viewer shows.
func (a *App) LicenceText() string {
	return embeddedLicence
}

// patchThumbnail returns the image a newly generated patch mod uses as its
// thumbnail: a patch_thumbnail.png the user has put in this app's config
// folder if there is a readable, non-empty one, otherwise the built-in image.
func (a *App) patchThumbnail() []byte {
	if a.patchThumbnailPath != "" {
		if data, err := os.ReadFile(a.patchThumbnailPath); err == nil && len(data) > 0 {
			return data
		}
	}
	return embeddedPatchThumbnail
}

// DefaultThumbnail is the app's own placeholder art - the same image a newly generated patch mod
// gets as its thumbnail (see patchThumbnail) - for anywhere else in the interface that needs a
// "no preview available" image instead of a plain empty box, such as the DLC detail panel for a
// pack Steam hasn't sent a header image for yet.
func (a *App) DefaultThumbnail() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(a.patchThumbnail())
}

// SetPatchOverride persists a manual winner override for one specific
// conflict, identified by its Type and ID (library.ConflictSummary's own
// fields) - modID must be one of that conflict's real candidates to take
// effect (an invalid one is stored but simply ignored by
// library.GeneratePatch and the next ScanGame's conflict summary, falling
// back to the automatic winner - see internal/library's effectiveWinner),
// or "" to clear a previous override and revert to that automatic
// winner. See docs/patch-mods.md.
func (a *App) SetPatchOverride(gameID, conflictType, conflictID, modID string) error {
	overrides := a.patchOverrides.Load(gameID)
	key := patchoverride.Key(conflictType, conflictID)
	if modID == "" {
		delete(overrides, key)
	} else {
		overrides[key] = modID
	}
	if err := a.patchOverrides.Save(gameID, overrides); err != nil {
		applog.For("Patch").Errorf("couldn't save the pick for '%s': %v", key, err)
		return err
	}
	if modID == "" {
		applog.For("Patch").Infof("'%s' goes back to its load-order winner", key)
	} else {
		applog.For("Patch").Infof("manual pick saved: mod '%s' wins '%s'", modID, key)
	}
	return nil
}

// ClearPatchOverrides removes every manual patch-override winner choice
// for gameID in one write, reverting every conflict back to its automatic
// load-order winner - the Conflict Resolver's "Auto-resolve all" action.
// A bulk version of SetPatchOverride(gameID, type, id, "") that writes the
// overrides file once instead of once per override.
func (a *App) ClearPatchOverrides(gameID string) error {
	if err := a.patchOverrides.Save(gameID, map[string]string{}); err != nil {
		applog.For("Patch").Errorf("couldn't clear the manual picks for '%s': %v", a.gameLabel(gameID), err)
		return err
	}
	applog.For("Patch").Infof("cleared every manual pick for '%s'", a.gameLabel(gameID))
	return nil
}

// IgnoredIncompatibleMods returns gameID's set of mod IDs whose
// version-incompatibility warning the user has chosen to suppress - see
// internal/versionignore.
func (a *App) IgnoredIncompatibleMods(gameID string) []string {
	return a.versionIgnore.Load(gameID)
}

// SetModIncompatibilityIgnored adds or removes modID from gameID's
// ignored-incompatibility set - the Workspace mod context menu's own
// "Ignore incompatibility"/"Stop ignoring incompatibility" pair.
func (a *App) SetModIncompatibilityIgnored(gameID, modID string, ignored bool) error {
	existing := a.versionIgnore.Load(gameID)
	set := make(map[string]bool, len(existing))
	for _, id := range existing {
		set[id] = true
	}
	if ignored {
		set[modID] = true
	} else {
		delete(set, modID)
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	if err := a.versionIgnore.Save(gameID, ids); err != nil {
		applog.For("Library").Errorf("couldn't save the ignored version warnings for '%s': %v", a.gameLabel(gameID), err)
		return err
	}
	if ignored {
		applog.For("Library").Infof("ignoring the game version warning for mod '%s' in '%s'", modID, a.gameLabel(gameID))
	} else {
		applog.For("Library").Infof("no longer ignoring the game version warning for mod '%s' in '%s'", modID, a.gameLabel(gameID))
	}
	return nil
}

// ResolvedConflicts returns gameID's real, currently-marked-resolved
// conflict keys (see internal/resolvedconflicts.Key) - the Conflict
// Resolver's own contested-keys list uses this to show a manually
// reviewed key in green instead of red.
func (a *App) ResolvedConflicts(gameID string) []string {
	return a.resolvedConflicts.Load(gameID)
}

// PinnedMods returns gameID's set of mod IDs pinned to the top of the mod list - see
// internal/modpins.
func (a *App) PinnedMods(gameID string) []string {
	return a.modPins.Load(gameID)
}

// SetModPinned adds or removes modID from gameID's pinned set - the Editor's and the Library's
// own "Pin"/"Unpin" context menu items.
func (a *App) SetModPinned(gameID, modID string, pinned bool) error {
	existing := a.modPins.Load(gameID)
	set := make(map[string]bool, len(existing))
	for _, id := range existing {
		set[id] = true
	}
	if pinned {
		set[modID] = true
	} else {
		delete(set, modID)
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	if err := a.modPins.Save(gameID, ids); err != nil {
		applog.For("Library").Errorf("couldn't save the pinned mods for '%s': %v", a.gameLabel(gameID), err)
		return err
	}
	if pinned {
		applog.For("Library").Infof("pinned mod '%s' in '%s'", modID, a.gameLabel(gameID))
	} else {
		applog.For("Library").Infof("unpinned mod '%s' in '%s'", modID, a.gameLabel(gameID))
	}
	return nil
}

// SetConflictResolved adds or removes one conflict (identified the same
// Type+ID way library.ConflictSummary itself is) from gameID's resolved
// set - the diff view's own "Mark done"/"Mark unresolved" toggle. Purely
// a personal bookkeeping flag: it never changes which mod actually wins
// that key (see internal/resolvedconflicts' own doc comment) - for that,
// see SetPatchOverride.
func (a *App) SetConflictResolved(gameID, conflictType, conflictID string, resolved bool) error {
	key := resolvedconflicts.Key(conflictType, conflictID)
	existing := a.resolvedConflicts.Load(gameID)
	set := make(map[string]bool, len(existing))
	for _, k := range existing {
		set[k] = true
	}
	if resolved {
		set[key] = true
	} else {
		delete(set, key)
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	if err := a.resolvedConflicts.Save(gameID, keys); err != nil {
		applog.For("Conflicts").Errorf("couldn't save the reviewed flag for '%s': %v", key, err)
		return err
	}
	if resolved {
		applog.For("Conflicts").Infof("'%s' marked as reviewed", key)
	} else {
		applog.For("Conflicts").Infof("'%s' marked as not reviewed", key)
	}
	return nil
}

// OpenModFolder opens modID's real content folder in the OS file manager.
func (a *App) OpenModFolder(gameID, modID string) error {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	path, err := library.ModFolderPath(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)}, modID)
	if err != nil {
		applog.For("Files").Warnf("couldn't find the folder of mod '%s': %v", modID, err)
		return err
	}
	if err := browser.OpenFile(path); err != nil {
		applog.For("Files").Warnf("couldn't open '%s': %v", path, err)
		return err
	}
	return nil
}

// ListPlaysets returns every saved playset's name for gameID.
func (a *App) ListPlaysets(gameID string) ([]string, error) {
	return a.playsets.List(a.ctx, gameID)
}

// ImportLauncherPlaysets reads gameID's real Paradox Launcher database
// (launcher-v2.sqlite), read-only, for the Workspace playset switcher's
// own "Import from Paradox Launcher" section - see internal/launcherdb and
// docs/launcher-database.md. A missing file is reported as an empty list,
// not an error: plenty of real installs have never had the real Launcher
// opened against them, and that's not a failure worth surfacing.
//
// Classic-descriptor games only, matching every other classic-only
// precedent in this codebase (EnsureStub, patch-mod generation) -
// launcher-v2.sqlite's schema is confirmed for this format; JSON-launcher
// games may use a differently-shaped database under the same filename,
// which this project has no confirmed schema for yet.
func (a *App) ImportLauncherPlaysets(gameID string) ([]launcherdb.Playset, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	if cfg.DescriptorType != mod.DescriptorClassic {
		return []launcherdb.Playset{}, nil
	}
	dir, err := cfg.UserDataDir()
	if err != nil {
		return nil, err
	}
	playsets, err := launcherdb.ListPlaysets(dir)
	if errors.Is(err, launcherdb.ErrNotFound) {
		applog.For("Playsets").Infof("no Paradox Launcher database found for '%s'", cfg.DisplayName)
		return []launcherdb.Playset{}, nil
	}
	if err != nil {
		applog.For("Playsets").Warnf("couldn't read the Paradox Launcher database for '%s': %v", cfg.DisplayName, err)
		return nil, err
	}
	applog.For("Playsets").Infof("found %d playsets in the Paradox Launcher database for '%s'", len(playsets), cfg.DisplayName)
	return playsets, nil
}

// ListCollections returns every saved collection, full data (not just
// names) - the Library screen's own sidebar shows each one's real mod
// count. Cross-game, unlike ListPlaysets - see internal/collection.
func (a *App) ListCollections() ([]collection.Collection, error) {
	return a.collections.List(a.ctx)
}

// SaveCollection creates or overwrites one named collection.
func (a *App) SaveCollection(c collection.Collection) error {
	if err := a.collections.Save(a.ctx, c); err != nil {
		applog.For("Collections").Errorf("couldn't save '%s': %v", c.Name, err)
		return err
	}
	applog.For("Collections").Infof("saved '%s'", c.Name)
	return nil
}

// DeleteCollection removes one named collection.
func (a *App) DeleteCollection(name string) error {
	if err := a.collections.Delete(a.ctx, name); err != nil {
		applog.For("Collections").Errorf("couldn't delete '%s': %v", name, err)
		return err
	}
	applog.For("Collections").Infof("deleted '%s'", name)
	return nil
}

// ListDLC lists gameID's real DLC, for the DLC screen's per-playset toggle
// list - both what's actually installed (toggleable) and, merged in from
// the last Steam Store refresh (see DLCStoreData), anything else the base
// game's own official Steam catalog lists but that isn't installed here
// (shown, never toggleable - see library.MergeDLCCatalog). A cache read
// failure here is non-fatal: the merge is best-effort, so an installed-
// only listing is still returned rather than failing the whole call.
func (a *App) ListDLC(gameID string) ([]dlc.Entry, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	local, err := library.ListDLC(cfg)
	if err != nil {
		return nil, err
	}
	if a.cacheDir == "" {
		return local, nil
	}
	cf, err := (dlcstore.Store{Dir: a.cacheDir, GameKey: gameID}).Load()
	if err != nil {
		return local, nil
	}
	return library.MergeDLCCatalog(local, cf), nil
}

// WorkshopDetails returns real Steam Workshop metadata (title,
// description, subscriber/view counts, last-updated time) for gameID's
// currently scanned Workshop mods, for the mod detail panel's Changes tab.
// Fetched once per mod in a single batched request and kept in memory for
// the rest of the app's runtime - see library.WorkshopDetailsCache.
//
// Returns a slice, not a map keyed by published file id (each entry's own
// ID field is that key) - a struct only ever reachable through a Go map's
// value type doesn't get its own TS class generated by this project's
// installed Wails version, unlike one reachable through a slice.
func (a *App) WorkshopDetails(gameID string) ([]steamapi.PublishedFileDetails, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	timer := applog.For("Steam").Begin()
	byID, err := a.workshopDetails.Get(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		timer.Warnf("couldn't get Workshop details for '%s': %v", cfg.DisplayName, err)
		return nil, err
	}
	timer.Infof("Workshop details for '%s': %d mods", cfg.DisplayName, len(byID))
	result := make([]steamapi.PublishedFileDetails, 0, len(byID))
	for _, d := range byID {
		result = append(result, d)
	}
	return result, nil
}

// AuthorProfiles returns real Steam Community profile data (display name,
// avatar, member-since date, profile link) for every distinct creator
// among gameID's currently scanned Workshop mods, for the mod detail
// panel's Changes tab. Reuses whatever WorkshopDetails has already
// fetched this runtime rather than re-fetching - a profile is only ever
// looked up for a creator this project already learned about from real
// Workshop data it fetched for its own purposes.
//
// Returns a slice of self-identifying AuthorProfile (SteamID + Profile),
// not a map keyed by SteamID64 - see WorkshopDetails' doc comment for the
// Wails codegen reason slices are used instead of maps here.
func (a *App) AuthorProfiles(gameID string) ([]library.AuthorProfile, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	timer := applog.For("Steam").Begin()
	byID, err := a.workshopDetails.Get(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		timer.Warnf("couldn't get Workshop details to find authors for '%s': %v", cfg.DisplayName, err)
		return nil, err
	}
	seen := make(map[string]bool, len(byID))
	steamIDs := make([]string, 0, len(byID))
	for _, d := range byID {
		if d.Creator != "" && !seen[d.Creator] {
			seen[d.Creator] = true
			steamIDs = append(steamIDs, d.Creator)
		}
	}
	profiles, err := a.authorProfiles.Get(a.ctx, steamIDs)
	if err != nil {
		timer.Warnf("couldn't get author profiles for '%s': %v", cfg.DisplayName, err)
		return nil, err
	}
	timer.Infof("author profiles for '%s': %d of %d creators", cfg.DisplayName, len(profiles), len(steamIDs))
	result := make([]library.AuthorProfile, 0, len(profiles))
	for id, p := range profiles {
		result = append(result, library.AuthorProfile{SteamID: id, Profile: p})
	}
	return result, nil
}

// ModChangelog returns a Workshop mod's real, most recent Steam update
// notes, for the mod detail panel's Changes tab. publishedFileID is the
// mod's own RemoteFileID, already known to the frontend from its scanned
// mod record - this doesn't need a game context of its own, unlike most
// bound methods here. Fetched once per id and kept in memory for the
// app's runtime - see library.ChangelogCache.
func (a *App) ModChangelog(publishedFileID string) ([]steamapi.ChangelogEntry, error) {
	if publishedFileID == "" {
		return nil, fmt.Errorf("app: empty published file id")
	}
	timer := applog.For("Steam").Begin()
	entries, err := a.changelogs.Get(a.ctx, publishedFileID)
	if err != nil {
		timer.Warnf("couldn't get the update notes for Workshop item '%s': %v", publishedFileID, err)
		return nil, err
	}
	timer.Infof("update notes for Workshop item '%s': %d entries", publishedFileID, len(entries))
	return entries, nil
}

// DLCStoreData returns gameID's cached real Steam Store data for its
// installed DLC (header image, price, short description) - see
// dlcstore.Store. Always returns immediately with whatever's on disk, even
// if stale; when the cache is missing or older than dlcstore.MaxAge, a
// background refresh is kicked off (never more than one at a time per
// game) and a "dlc-store-refreshed" event fires once it's saved, so the
// frontend can re-fetch when fresher data is actually ready rather than
// blocking this call on a live Steam round-trip.
//
// Returns a slice, not a map keyed by DLC id (each entry's own ID field is
// that key) - see WorkshopDetails' doc comment for why.
func (a *App) DLCStoreData(gameID string) ([]dlcstore.StoreData, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}

	store := dlcstore.Store{Dir: a.cacheDir, GameKey: gameID}
	cf, err := store.Load()
	if err != nil {
		return nil, err
	}

	if a.cacheDir != "" && store.NeedsRefresh(cf) {
		a.startDLCStoreRefresh(cfg, gameID, store)
	}
	result := make([]dlcstore.StoreData, 0, len(cf.ByAppID))
	for _, d := range cf.ByAppID {
		result = append(result, d)
	}
	return result, nil
}

// startDLCStoreRefresh kicks off a background Steam Store refresh for
// gameID, unless one is already in flight.
func (a *App) startDLCStoreRefresh(cfg game.GameConfig, gameID string, store dlcstore.Store) {
	a.dlcRefreshingMu.Lock()
	if a.dlcRefreshing == nil {
		a.dlcRefreshing = map[string]bool{}
	}
	if a.dlcRefreshing[gameID] {
		a.dlcRefreshingMu.Unlock()
		return
	}
	a.dlcRefreshing[gameID] = true
	a.dlcRefreshingMu.Unlock()

	go func() {
		defer func() {
			a.dlcRefreshingMu.Lock()
			delete(a.dlcRefreshing, gameID)
			a.dlcRefreshingMu.Unlock()
		}()

		timer := applog.For("Steam").Begin()
		applog.For("Steam").Infof("refreshing the DLC store data for '%s'", cfg.DisplayName)
		entries, err := library.ListDLC(cfg)
		if err != nil {
			timer.Warnf("couldn't list the DLC for '%s': %v", cfg.DisplayName, err)
			return
		}
		cf := dlcstore.Refresh(a.ctx, cfg.SteamAppID, entries)
		saved, err := store.SaveRefreshed(cf)
		if err != nil {
			timer.Warnf("couldn't save the DLC store data for '%s': %v", cfg.DisplayName, err)
			return
		}
		if !saved {
			// SaveRefreshed keeps the old data when a refresh comes back empty
			// (Steam unreachable, say) rather than overwriting it with nothing.
			timer.Warnf("the DLC store refresh for '%s' returned nothing, so the existing data was kept", cfg.DisplayName)
			return
		}
		timer.Infof("DLC store data for '%s' refreshed (%d entries)", cfg.DisplayName, len(cf.ByAppID))
		wailsruntime.EventsEmit(a.ctx, "dlc-store-refreshed", gameID)
	}()
}

// LoadPlayset returns one saved playset.
func (a *App) LoadPlayset(gameID, name string) (playset.Playset, error) {
	p, err := a.playsets.Load(a.ctx, gameID, name)
	if err != nil {
		applog.For("Playsets").Errorf("couldn't load '%s' for '%s': %v", name, a.gameLabel(gameID), err)
		return p, err
	}
	applog.For("Playsets").Infof("loaded '%s' for '%s' (%d mods)", name, a.gameLabel(gameID), len(p.ModIDs))
	return p, nil
}

// SavePlayset persists a playset (creating or overwriting by name).
func (a *App) SavePlayset(p playset.Playset) error {
	if err := a.playsets.Save(a.ctx, p); err != nil {
		applog.For("Playsets").Errorf("couldn't save '%s': %v", p.Name, err)
		return err
	}
	applog.For("Playsets").Infof("saved '%s' for '%s' (%d mods)", p.Name, a.gameLabel(p.GameKey), len(p.ModIDs))
	return nil
}

// DeletePlayset removes a saved playset, and forgets it in settings: a game
// whose last-active or pinned auto-load playset was this one falls back to
// starting blank, instead of trying to load a playset that's gone.
func (a *App) DeletePlayset(gameID, name string) error {
	if err := a.playsets.Delete(a.ctx, gameID, name); err != nil {
		applog.For("Playsets").Errorf("couldn't delete '%s': %v", name, err)
		return err
	}
	applog.For("Playsets").Infof("deleted '%s' for '%s'", name, a.gameLabel(gameID))
	if err := a.mutatePreferences(func(p preferences.Preferences) preferences.Preferences {
		return p.WithoutPlayset(gameID, name)
	}); err != nil {
		// The playset is gone; only the settings that mention it couldn't be
		// tidied, which the app already treats as "unset" when it loads.
		applog.For("Playsets").Warnf("deleted '%s' but couldn't update the settings that referred to it: %v", name, err)
	}
	return nil
}

// RenamePlayset gives a saved playset a new name, keeping its mods, load order
// and DLC choices, and points the settings that refer to it by name (the last
// active playset, the one pinned for auto-loading) at the new name. Refuses an
// empty name, and a name another playset already has. The errors are written to
// be shown to the person.
func (a *App) RenamePlayset(gameID, oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	err := a.playsets.Rename(a.ctx, gameID, oldName, newName)
	switch {
	case errors.Is(err, playset.ErrInvalidName):
		return fmt.Errorf("give the playset a name")
	case errors.Is(err, playset.ErrExists):
		applog.For("Playsets").Warnf("can't rename '%s' to '%s': a playset with that name already exists", oldName, newName)
		return fmt.Errorf("a playset named %q already exists", newName)
	case errors.Is(err, playset.ErrNotFound):
		return fmt.Errorf("that playset no longer exists")
	case err != nil:
		applog.For("Playsets").Errorf("couldn't rename '%s' to '%s': %v", oldName, newName, err)
		return err
	}
	applog.For("Playsets").Infof("renamed '%s' to '%s' for '%s'", oldName, newName, a.gameLabel(gameID))
	if err := a.mutatePreferences(func(p preferences.Preferences) preferences.Preferences {
		return p.WithPlaysetRenamed(gameID, oldName, newName)
	}); err != nil {
		applog.For("Playsets").Warnf("renamed '%s' but couldn't update the settings that referred to it: %v", oldName, err)
	}
	return nil
}

// mutatePreferences applies change to the current preferences and saves the
// result (through SetPreferences, so it's logged and the watcher refreshed).
func (a *App) mutatePreferences(change func(preferences.Preferences) preferences.Preferences) error {
	a.preferencesMu.Lock()
	next := change(a.preferences)
	a.preferencesMu.Unlock()
	return a.savePreferences(next, false)
}

// CheckGameUpdates compares each managed game's installed version with the one
// last seen and returns the games that changed - a game update, or a rollback -
// remembering the new versions so each is reported once. A game seen for the
// first time is recorded quietly (there was nothing to update from), and one
// whose version can't be read is skipped rather than treated as a change. The
// frontend calls it at startup and when the window regains focus, since Steam
// may well have updated the game while this app was open.
func (a *App) CheckGameUpdates() ([]library.GameUpdate, error) {
	a.preferencesMu.Lock()
	managed := make(map[string]bool, len(a.preferences.ManagedGames))
	for _, id := range a.preferences.ManagedGames {
		managed[id] = true
	}
	a.preferencesMu.Unlock()

	current := map[string]string{}
	names := map[string]string{}
	for _, cfg := range a.registry.List() {
		if len(managed) > 0 && !managed[cfg.ID] {
			continue
		}
		version, _ := a.GameVersion(cfg.ID)
		current[cfg.ID] = version
		names[cfg.ID] = cfg.DisplayName
	}

	a.preferencesMu.Lock()
	next, changes := a.preferences.ObserveGameVersions(current)
	same := reflect.DeepEqual(next.LastSeenGameVersions, a.preferences.LastSeenGameVersions)
	a.preferencesMu.Unlock()

	if !same {
		if err := a.savePreferences(next, false); err != nil {
			applog.For("Games").Warnf("couldn't record the installed game versions: %v", err)
		}
	}
	updates := make([]library.GameUpdate, 0, len(changes))
	for _, c := range changes {
		applog.For("Games").Infof("'%s' changed from '%s' to '%s'", names[c.GameID], c.From, c.To)
		updates = append(updates, library.GameUpdate{GameID: c.GameID, GameName: names[c.GameID], From: c.From, To: c.To})
	}
	return updates, nil
}

// LaunchGame writes the named playset's dlc_load.json and launches the game
// via the real OSLauncher - this is the one method in this app that opens
// Steam and starts the actual game process.
//
// playsetName == "" is a deliberate, supported case, not a missing
// argument: Play is never disabled just because no playset is loaded (see
// Workspace.tsx's play-button) - launching with nothing selected here
// skips loading a playset and every one of this function's own state
// writes entirely, leaving dlc_load.json/mods_registry.json/game_data.json
// exactly as they already are and launching the game against that as-is.
// That's either whatever this app last wrote for a real playset, or
// whatever Steam/the Paradox Launcher last wrote before this app ever
// touched the game - never a state this function invents itself.
func (a *App) LaunchGame(gameID, playsetName string) error {
	log := applog.For("Launch")
	timer := log.Begin()
	if playsetName != "" {
		log.Infof("launching '%s' with playset '%s'", a.gameLabel(gameID), playsetName)
	} else {
		log.Infof("launching '%s' with whatever is already on disk", a.gameLabel(gameID))
	}
	if err := a.launchGame(gameID, playsetName); err != nil {
		timer.Errorf("launching '%s' failed: %v", a.gameLabel(gameID), err)
		return err
	}
	timer.Infof("'%s' launched", a.gameLabel(gameID))
	return nil
}

// launchGame is LaunchGame without the logging around it.
func (a *App) launchGame(gameID, playsetName string) error {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}

	if playsetName != "" {
		p, err := a.playsets.Load(a.ctx, gameID, playsetName)
		if err != nil {
			return err
		}

		scanResult, err := scan.Scan(a.ctx, scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
		if err != nil {
			return err
		}

		stateDir, err := cfg.UserDataDir()
		if err != nil {
			return err
		}

		// The game finds each enabled mod through its stub in game/mod/ (dlc_load.json's
		// "mod/<id>.mod" entries), not through the scan this app just did: a Workshop
		// item found without a stub, a mod in an extra mod folder, or a stub whose folder
		// moved would all look fine here and never load. Make sure every enabled mod has a
		// stub that points at where the mod really is. Only classic-descriptor games use
		// this stub convention.
		if cfg.DescriptorType == mod.DescriptorClassic {
			a.ensureModStubs(gameID, filepath.Join(stateDir, "mod"), p.ModIDs, scanResult.Mods)
		}

		order := conflict.LoadOrder(p.ModIDs)
		written, err := launch.WriteState(order, scanResult.Mods, cfg, launch.Options{
			StateDir:    stateDir,
			DisabledDLC: p.DisabledDLC,
		})
		if err != nil {
			return err
		}
		applog.For("Launch").Infof("wrote the launcher state for '%s': %d mods in the load order, %d DLC disabled, %d files", cfg.DisplayName, len(order), len(p.DisabledDLC), len(written.Written))
	}

	opts, err := a.launchOptionsFor(cfg)
	if err != nil {
		return err
	}
	if opts.Direct {
		applog.For("Launch").Infof("starting the game executable directly, without the Paradox Launcher")
	} else {
		applog.For("Launch").Infof("starting through Steam")
	}
	if err := launch.Launch(launch.OSLauncher{}, cfg, opts); err != nil {
		return err
	}

	if a.preferences.CloseAfterLaunch {
		wailsruntime.Quit(a.ctx)
	}
	return nil
}
