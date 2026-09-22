package main

import (
	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/cache"
	"github.com/Official-Husko/parallax-mod-manager/internal/modcheck"
)

// CheckResult is what the Editor's Checks tab shows for one mod.
type CheckResult struct {
	Findings []modcheck.Finding
	// BaseGameChecked is false when the game's install directory couldn't
	// be found, so the tab can say why that category came back empty
	// instead of implying a clean result it never actually checked.
	BaseGameChecked bool
}

// CheckMod runs every check the Editor's Checks tab shows for one mod:
// syntax errors in its own script files, conflicts with the base game, a
// descriptor problem, and a dependency that isn't installed. See
// internal/modcheck. installedNames mirrors what the Edit tab's own
// dependency field already gets from the frontend's own mod list, so both
// flag an unknown dependency the same way.
func (a *App) CheckMod(gameID, modID string, installedNames []string) (CheckResult, error) {
	t, err := a.editTarget(a.baseContext(), gameID, modID)
	if err != nil {
		return CheckResult{}, err
	}

	installDir, _ := a.resolveInstallDir(t.cfg)

	findings, err := modcheck.Check(a.baseContext(), t.m, t.cfg, modcheck.Options{
		Store:          cache.FileStore{Dir: a.cacheDir},
		InstallDir:     installDir,
		InstalledNames: installedNames,
	})
	if err != nil {
		applog.For("Checks").Warnf("checking '%s' in '%s' failed: %v", editLabel(t), t.cfg.DisplayName, err)
		return CheckResult{}, err
	}
	return CheckResult{Findings: findings, BaseGameChecked: installDir != ""}, nil
}
