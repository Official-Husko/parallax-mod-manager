package app

import (
	"encoding/json"
	"fmt"
	"time"

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
	// FilesRead is how many of this mod's own files this run examined
	// (cached ones included) - the Result cache card's own "This mod" stat.
	FilesRead int
	// ResultBytes is a rough size of this run's own findings, purely
	// informational (findings are never actually persisted anywhere - there
	// is no on-disk "result cache" file to report the real size of).
	ResultBytes int64
	// DurationMS and RanAt (unix seconds) are this run's own timing.
	DurationMS int64
	RanAt      int64
	// BaseGameIndexFiles/BaseGameIndexBytes describe the base game's own
	// cached index on disk (see modcheck.VanillaModID) - 0/0 when the game
	// was never checked, or its index could not be read.
	BaseGameIndexFiles int
	BaseGameIndexBytes int64
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
	store := cache.FileStore{Dir: a.cacheDir}

	start := time.Now()
	findings, filesRead, err := modcheck.Check(a.baseContext(), t.m, t.cfg, modcheck.Options{
		Store:          store,
		InstallDir:     installDir,
		InstalledNames: installedNames,
	})
	if err != nil {
		applog.For("Checks").Warnf("checking '%s' in '%s' failed: %v", editLabel(t), t.cfg.DisplayName, err)
		return CheckResult{}, err
	}
	duration := time.Since(start)

	result := CheckResult{
		Findings:        findings,
		BaseGameChecked: installDir != "",
		FilesRead:       filesRead,
		DurationMS:      duration.Milliseconds(),
		RanAt:           time.Now().Unix(),
	}
	if data, marshalErr := json.Marshal(findings); marshalErr == nil {
		result.ResultBytes = int64(len(data))
	}
	if installDir != "" {
		if files, size, ok := store.Stat(t.cfg.ID, modcheck.VanillaModID); ok {
			result.BaseGameIndexFiles, result.BaseGameIndexBytes = files, size
		}
	}
	return result, nil
}

// RebuildBaseGameIndex deletes the base game's own cached index for gameID (see
// modcheck.VanillaModID), so the next Checks run reparses it from scratch - the same recovery a
// person could get by deleting the cache file by hand, exposed as a button instead.
func (a *App) RebuildBaseGameIndex(gameID string) error {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return fmt.Errorf("app: unknown game %q", gameID)
	}
	return cache.FileStore{Dir: a.cacheDir}.Remove(cfg.ID, modcheck.VanillaModID)
}
