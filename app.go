package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/gamemedia"
	"github.com/Official-Husko/parallax-mod-manager/internal/launch"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/playset"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

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
	gameMedia     gamemedia.Store
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
		d, err := library.DetectGame(a.ctx, cfg)
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
		return library.DetectGame(a.ctx, cfg)
	}
	if !cfg.VerifyInstallDir(dir) {
		return library.DetectedGame{}, fmt.Errorf("app: %s does not look like a %s install", dir, cfg.DisplayName)
	}
	return library.DetectGameAt(a.ctx, cfg, dir)
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
			return library.DetectGameAt(a.ctx, cfg, dir)
		}
	}
	return library.DetectedGame{}, fmt.Errorf("app: %s does not match any registered game", dir)
}

// ScanGame scans, parses, and resolves conflicts for one supported game.
// playsetName is optional; empty means every mod enabled, ID-sorted (no
// selection made yet).
func (a *App) ScanGame(gameID, playsetName string) (library.Summary, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return library.Summary{}, fmt.Errorf("app: unknown game %q", gameID)
	}

	opts := library.Options{CacheDir: a.cacheDir}
	if playsetName != "" {
		p, err := a.playsets.Load(a.ctx, gameID, playsetName)
		if err != nil {
			return library.Summary{}, err
		}
		opts.Order = conflict.LoadOrder(p.ModIDs)
	}
	return library.LoadGame(a.ctx, cfg, opts)
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

	scanResult, err := scan.Scan(a.ctx, scan.Options{Game: cfg})
	if err != nil {
		return err
	}

	stateDir, err := cfg.UserDataDir()
	if err != nil {
		return err
	}
	order := conflict.LoadOrder(p.ModIDs)
	if _, err := launch.WriteState(order, scanResult.Mods, cfg, launch.Options{
		StateDir:    stateDir,
		DisabledDLC: p.DisabledDLC,
	}); err != nil {
		return err
	}

	return launch.Launch(launch.OSLauncher{}, cfg, launch.LaunchOptions{})
}
