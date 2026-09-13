package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/browser"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/gamemedia"
	"github.com/Official-Husko/parallax-mod-manager/internal/launch"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/playset"
	"github.com/Official-Husko/parallax-mod-manager/internal/preferences"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
	"github.com/Official-Husko/parallax-mod-manager/internal/steam"
	"github.com/Official-Husko/parallax-mod-manager/internal/watch"
)

// modWatchDebounce absorbs a burst of filesystem events from one logical
// change (a bulk copy, an archive extract) into a single refresh.
const modWatchDebounce = 400 * time.Millisecond

// App is the Wails-bound backend: a thin adapter that resolves real OS
// paths (the one place this project's "never resolve a real path inside a
// package that might be tested" rule doesn't apply, per CLAUDE.md) and
// delegates everything else to internal/ packages.
type App struct {
	ctx             context.Context
	registry        *game.Registry
	startupNotice   string
	cacheDir        string
	playsets        playset.Store
	gameMedia       gamemedia.Store
	preferences     preferences.Preferences
	preferencesPath string
	steamRoots      []string
	modWatcher      *watch.FolderWatcher
	watchedGameID   string
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
	if configErr == nil {
		a.playsets = playset.FileStore{Dir: filepath.Join(configDir, "parallax-mod-manager", "playsets")}
	}

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
}

// shutdown is called when the app is closing, before the runtime exits.
func (a *App) shutdown(ctx context.Context) {
	a.modWatcher.Close()
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

// GetPreferences returns the app's current preferences.
func (a *App) GetPreferences() preferences.Preferences {
	return a.preferences
}

// SetPreferences persists p and applies it immediately: if scanning for
// new mods was just turned on or off, the live watcher for whichever game
// is currently being watched (see WatchMods) is started or stopped right
// away rather than waiting for the next game switch.
func (a *App) SetPreferences(p preferences.Preferences) error {
	if a.preferencesPath != "" {
		if err := preferences.Save(a.preferencesPath, p); err != nil {
			return err
		}
	}
	a.preferences = p

	if a.watchedGameID != "" {
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
		wailsruntime.EventsEmit(a.ctx, "mods-changed", gameID)
	})
	if err != nil {
		return err
	}
	a.modWatcher = w
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
// state, for the first-run wizard's "games found" step.
func (a *App) DetectGames() ([]library.DetectedGame, error) {
	games := a.registry.List()
	result := make([]library.DetectedGame, 0, len(games))
	for _, cfg := range games {
		d, err := library.DetectGame(a.ctx, cfg, a.steamRoots)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, nil
}

// BrowseForGameInstall opens a native folder-picker so a user can point at
// a game install automatic Steam-library detection didn't find (a non-Steam
// copy, an unusual library setup). Returns the game's current detected
// state unchanged if the user cancels the dialog (an empty path is not an
// error); returns an error if a folder was chosen but doesn't actually
// contain the game (verified against SignatureFiles) - never trusts an
// unverified folder just because the user picked it.
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
		return library.DetectGame(a.ctx, cfg, a.steamRoots)
	}
	if !cfg.VerifyInstallDir(dir) {
		return library.DetectedGame{}, fmt.Errorf("app: %s does not look like a %s install", dir, cfg.DisplayName)
	}
	return library.DetectGameAt(a.ctx, cfg, dir, a.steamRoots)
}

// BrowseForAnyGameInstall opens the same folder-picker as
// BrowseForGameInstall, but without a specific game in mind: the chosen
// folder is checked against every registered game, and whichever one it
// verifies against is returned. Useful when a game wasn't auto-detected and
// the user isn't sure (or doesn't want to hunt for) which row to browse
// from. Returns a zero-value DetectedGame (empty ID) if the user cancels;
// errors only if a folder was chosen but matches no registered game.
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
		if cfg.VerifyInstallDir(dir) {
			return library.DetectGameAt(a.ctx, cfg, dir, a.steamRoots)
		}
	}
	return library.DetectedGame{}, fmt.Errorf("app: %s does not match any registered game", dir)
}

// ScanGame scans, parses, and resolves conflicts for one supported game.
// playsetName is optional; empty means no selection made yet - every
// scanned mod starts disabled.
func (a *App) ScanGame(gameID, playsetName string) (library.Summary, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.Summary{}, fmt.Errorf("app: unknown game %q", gameID)
	}

	opts := library.Options{
		CacheDir:   a.cacheDir,
		SteamRoots: a.steamRoots,
		// The mod list itself (names/versions/sources) is known the moment
		// scanning finishes, well before conflict detection's slower
		// per-mod content parsing completes - emit it immediately so the
		// frontend can show the list right away instead of blocking on a
		// full "Scanning..." page.
		OnQuickSummary: func(s library.Summary) {
			wailsruntime.EventsEmit(a.ctx, "scan-quick", gameID, s)
		},
	}
	if playsetName != "" {
		p, err := a.playsets.Load(a.ctx, gameID, playsetName)
		if err != nil {
			return library.Summary{}, err
		}
		opts.Order = conflict.LoadOrder(p.ModIDs)
	}
	return library.LoadGame(a.ctx, cfg, opts)
}

// ListModFiles returns modID's real on-disk file tree, for the Workspace
// detail panel's Files tab.
func (a *App) ListModFiles(gameID, modID string) (library.ModFiles, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.ModFiles{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.ListModFiles(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots}, modID)
}

// ModThumbnail returns modID's real thumbnail image, if it has a usable
// one, as a data: URI - the same convention as GameMedia. Empty string, no
// error, means the mod has no thumbnail. See library.ModThumbnail.
func (a *App) ModThumbnail(gameID, modID string) (string, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return "", fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.ModThumbnail(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots}, modID)
}

// ModSizes returns every one of gameID's scanned mods' real on-disk content
// size (mod ID -> bytes), for the Library table's Size column.
func (a *App) ModSizes(gameID string) (map[string]int64, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.ModSizes(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots})
}

// ReadModFile returns one real file's text content from inside modID's
// content directory, for the conflict resolver's side-by-side view.
func (a *App) ReadModFile(gameID, modID, relPath string) (string, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return "", fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.ReadModFile(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots}, modID, relPath)
}

// FindEmptyMods lists gameID's real local mods with no usable content, for
// the Workspace's "Purge empty" review dialog. See library.FindEmptyMods.
func (a *App) FindEmptyMods(gameID string) ([]library.EmptyModCandidate, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.FindEmptyMods(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots})
}

// PurgeMods deletes the descriptor file for each of modIDs, after the user
// has reviewed and confirmed them in the "Purge empty" dialog. See
// library.PurgeMods.
func (a *App) PurgeMods(gameID string, modIDs []string) (library.PurgeResult, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.PurgeResult{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.PurgeMods(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots}, modIDs)
}

// GeneratePatch resolves every genuine conflict in gameID's current mod
// set (order, exactly as the caller's own in-memory load order - not a
// saved playset, since that's what the Conflict Resolver the user is
// looking at was actually computed from) and writes a real patch mod
// pinning down each one's winner. See library.GeneratePatch.
func (a *App) GeneratePatch(gameID string, order []string) (library.PatchResult, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.PatchResult{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	return library.GeneratePatch(a.ctx, cfg, library.Options{
		SteamRoots: a.steamRoots,
		Order:      conflict.LoadOrder(order),
	})
}

// OpenModFolder opens modID's real content folder in the OS file manager.
func (a *App) OpenModFolder(gameID, modID string) error {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	path, err := library.ModFolderPath(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots}, modID)
	if err != nil {
		return err
	}
	return browser.OpenFile(path)
}

// ListPlaysets returns every saved playset's name for gameID.
func (a *App) ListPlaysets(gameID string) ([]string, error) {
	return a.playsets.List(a.ctx, gameID)
}

// LoadPlayset returns one saved playset.
func (a *App) LoadPlayset(gameID, name string) (playset.Playset, error) {
	return a.playsets.Load(a.ctx, gameID, name)
}

// SavePlayset persists a playset (creating or overwriting by name).
func (a *App) SavePlayset(p playset.Playset) error {
	return a.playsets.Save(a.ctx, p)
}

// DeletePlayset removes a saved playset.
func (a *App) DeletePlayset(gameID, name string) error {
	return a.playsets.Delete(a.ctx, gameID, name)
}

// LaunchGame writes the named playset's dlc_load.json and launches the game
// via the real OSLauncher - this is the one method in this app that opens
// Steam and starts the actual game process.
func (a *App) LaunchGame(gameID, playsetName string) error {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}

	p, err := a.playsets.Load(a.ctx, gameID, playsetName)
	if err != nil {
		return err
	}

	scanResult, err := scan.Scan(a.ctx, scan.Options{Game: cfg, SteamRoots: a.steamRoots})
	if err != nil {
		return err
	}

	stateDir, err := cfg.UserDataDir()
	if err != nil {
		return err
	}

	// Some subscribed Workshop mods may have been discovered without a
	// game/mod/ linking stub yet (see scan.discoverUnlinkedWorkshopItems) -
	// write one for anything this playset actually enables, so the game
	// itself (which reads dlc_load.json's "mod/ugc_<id>.mod" entries) can
	// find it. Only classic-descriptor games use this stub convention.
	if cfg.DescriptorType == mod.DescriptorClassic {
		modsByID := make(map[string]mod.Mod, len(scanResult.Mods))
		for _, m := range scanResult.Mods {
			modsByID[m.ID] = m
		}
		modDir := filepath.Join(stateDir, "mod")
		for _, id := range p.ModIDs {
			if m, ok := modsByID[id]; ok {
				if _, err := scan.EnsureWorkshopStub(m, modDir); err != nil {
					return err
				}
			}
		}
	}

	order := conflict.LoadOrder(p.ModIDs)
	if _, err := launch.WriteState(order, scanResult.Mods, cfg, launch.Options{
		StateDir:    stateDir,
		DisabledDLC: p.DisabledDLC,
	}); err != nil {
		return err
	}

	if err := launch.Launch(launch.OSLauncher{}, cfg, launch.LaunchOptions{}); err != nil {
		return err
	}

	if a.preferences.CloseAfterLaunch {
		wailsruntime.Quit(a.ctx)
	}
	return nil
}
