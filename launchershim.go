package main

import (
	"errors"
	"fmt"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/launchershim"
)

// LauncherShimStatus is what Settings' Launch Options panel shows for "Steam
// Direct" - whether it's even offered for this game yet (see
// game.GameConfig.LauncherShimSupported), the real, on-disk install state (see
// launchershim.State), and the shim's own most recent reported run, if any (see
// launchershim.ReadStatus - this is account-wide, not per-game, since only one
// shim process ever runs at a time on one machine).
type LauncherShimStatus struct {
	Supported bool
	// State is one of launchershim's own State values ("not-installed",
	// "installed", "needs-repair", "unknown"), or "" if Supported is false or the
	// install directory couldn't be found (see Error).
	State string
	// Error explains why State is "" when it should have been resolvable - the
	// install directory not being found, most commonly. Never set just because
	// Supported is false; that's a normal, expected state of its own.
	Error   string
	HasRun  bool
	LastRun launchershim.Status
}

// LauncherShimStatusFor reports gameID's current Steam Direct status - see
// LauncherShimStatus.
func (a *App) LauncherShimStatusFor(gameID string) (LauncherShimStatus, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return LauncherShimStatus{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	out := LauncherShimStatus{Supported: cfg.LauncherShimSupported}

	if status, found, err := launchershim.ReadStatus(a.configAppDir); err == nil && found {
		out.HasRun = true
		out.LastRun = status
	}
	if !cfg.LauncherShimSupported {
		return out, nil
	}

	installDir, ok := a.resolveInstallDir(cfg)
	if !ok {
		out.Error = fmt.Sprintf("%s's install directory couldn't be found - set its path under Paths & folders", cfg.DisplayName)
		return out, nil
	}
	state, err := launchershim.Detect(installDir)
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	out.State = string(state)
	return out, nil
}

// InstallLauncherShim installs (or, over a launchershim.StateNeedsRepair install,
// repairs - Install already does the right thing for either starting state on its
// own) the Steam Direct shim in place of gameID's own dowser/dowser.exe.
func (a *App) InstallLauncherShim(gameID string) error {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	if !cfg.LauncherShimSupported {
		return errors.New("Steam Direct isn't confirmed working for this game yet")
	}
	installDir, ok := a.resolveInstallDir(cfg)
	if !ok {
		return fmt.Errorf("%s's install directory couldn't be found - set its path under Paths & folders", cfg.DisplayName)
	}
	if err := launchershim.Install(installDir); err != nil {
		applog.For("LauncherShim").Warnf("installing the shim for '%s' failed: %v", cfg.DisplayName, err)
		return err
	}
	applog.For("LauncherShim").Infof("installed the Steam Direct shim for '%s'", cfg.DisplayName)
	return nil
}

// RemoveLauncherShim restores gameID's real dowser/dowser.exe from its backup,
// undoing InstallLauncherShim.
func (a *App) RemoveLauncherShim(gameID string) error {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	installDir, ok := a.resolveInstallDir(cfg)
	if !ok {
		return fmt.Errorf("%s's install directory couldn't be found - set its path under Paths & folders", cfg.DisplayName)
	}
	if err := launchershim.Remove(installDir); err != nil {
		applog.For("LauncherShim").Warnf("removing the shim for '%s' failed: %v", cfg.DisplayName, err)
		return err
	}
	applog.For("LauncherShim").Infof("removed the Steam Direct shim for '%s'", cfg.DisplayName)
	return nil
}
