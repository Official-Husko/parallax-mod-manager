package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
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
	ctx      context.Context
	registry *game.Registry
	cacheDir string
	playsets playset.Store
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{registry: game.NewRegistry()}
}

// startup is called when the app starts. The context is saved so we can
// call the runtime methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if dir, err := os.UserCacheDir(); err == nil {
		a.cacheDir = filepath.Join(dir, "parallax-mod-manager")
	}
	if dir, err := os.UserConfigDir(); err == nil {
		a.playsets = playset.FileStore{Dir: filepath.Join(dir, "parallax-mod-manager", "playsets")}
	}
}

// ListGames returns every game this build supports, for a game picker.
func (a *App) ListGames() []library.GameInfo {
	games := a.registry.List()
	infos := make([]library.GameInfo, len(games))
	for i, g := range games {
		infos[i] = library.GameInfo{Key: g.Key, DisplayName: g.DisplayName}
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

// ScanGame scans, parses, and resolves conflicts for one supported game.
// playsetName is optional; empty means every mod enabled, ID-sorted (no
// selection made yet).
func (a *App) ScanGame(gameKey, playsetName string) (library.Summary, error) {
	cfg, ok := a.registry.Get(gameKey)
	if !ok {
		return library.Summary{}, fmt.Errorf("app: unknown game %q", gameKey)
	}

	opts := library.Options{CacheDir: a.cacheDir}
	if playsetName != "" {
		p, err := a.playsets.Load(a.ctx, gameKey, playsetName)
		if err != nil {
			return library.Summary{}, err
		}
		opts.Order = conflict.LoadOrder(p.ModIDs)
	}
	return library.LoadGame(a.ctx, cfg, opts)
}

// ListPlaysets returns every saved playset's name for gameKey.
func (a *App) ListPlaysets(gameKey string) ([]string, error) {
	return a.playsets.List(a.ctx, gameKey)
}

// LoadPlayset returns one saved playset.
func (a *App) LoadPlayset(gameKey, name string) (playset.Playset, error) {
	return a.playsets.Load(a.ctx, gameKey, name)
}

// SavePlayset persists a playset (creating or overwriting by name).
func (a *App) SavePlayset(p playset.Playset) error {
	return a.playsets.Save(a.ctx, p)
}

// DeletePlayset removes a saved playset.
func (a *App) DeletePlayset(gameKey, name string) error {
	return a.playsets.Delete(a.ctx, gameKey, name)
}

// LaunchGame writes the named playset's dlc_load.json and launches the game
// via the real OSLauncher - this is the one method in this app that opens
// Steam and starts the actual game process.
func (a *App) LaunchGame(gameKey, playsetName string) error {
	cfg, ok := a.registry.Get(gameKey)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameKey)
	}

	p, err := a.playsets.Load(a.ctx, gameKey, playsetName)
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
