package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/checksum"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// Checksum statuses, as PlaysetChecksum reports them.
const (
	// checksumReady: Checksum holds the calculated code.
	checksumReady = "ready"
	// checksumUnavailable: it could not be calculated; Reason says why.
	checksumUnavailable = "unavailable"
	// checksumUnsupported: this game has no known checksum scheme, so there is nothing to show.
	checksumUnsupported = "unsupported"
)

// PlaysetChecksumResult is the multiplayer checksum of a saved playset.
type PlaysetChecksumResult struct {
	Status string
	// Checksum is the four characters the game shows on its main menu.
	Checksum string
	// Files is how many files went into it, Mods how many enabled mods.
	Files int
	Mods  int
	// Reason says, in words for the user, why there is no checksum.
	Reason string
	// Warnings are things worth knowing that did not stop the calculation.
	Warnings []string
}

// PlaysetChecksum calculates the checksum the game will show for the named saved playset,
// without starting the game: the code every player in a multiplayer game has to share.
//
// It describes the playset as saved (what launching loads), not unsaved edits, and is meant to be
// asked for again whenever the playset is saved or loaded. A newer request for the same game
// stops an older one still running; the stopped one returns an error the caller can ignore.
func (a *App) PlaysetChecksum(gameID, playsetName string) (PlaysetChecksumResult, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return PlaysetChecksumResult{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	if !checksum.Algorithm(cfg.ChecksumAlgorithm).Known() {
		return PlaysetChecksumResult{Status: checksumUnsupported}, nil
	}

	ctx, done := a.startChecksum(gameID)
	defer done()

	log := applog.For("Checksum")
	started := time.Now()
	res, err := a.playsetChecksum(ctx, cfg, playsetName)
	if err != nil {
		if ctx.Err() != nil {
			return PlaysetChecksumResult{}, ctx.Err()
		}
		return PlaysetChecksumResult{}, err
	}
	took := time.Since(started).Round(time.Millisecond)
	switch res.Status {
	case checksumReady:
		log.Infof("'%s' with playset '%s': %s (%d files, %d mods, %s)", cfg.DisplayName, playsetName, res.Checksum, res.Files, res.Mods, took)
		for _, w := range res.Warnings {
			log.Warnf("%s", w)
		}
	default:
		log.Warnf("'%s' with playset '%s': no checksum - %s", cfg.DisplayName, playsetName, res.Reason)
	}
	return res, nil
}

// startChecksum registers a new calculation for gameID, cancelling the one still running for
// it. done releases the registration.
func (a *App) startChecksum(gameID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(a.baseContext())
	a.checksumMu.Lock()
	if a.checksumCancel == nil {
		a.checksumCancel = map[string]context.CancelFunc{}
	}
	if prev := a.checksumCancel[gameID]; prev != nil {
		prev()
	}
	a.checksumCancel[gameID] = cancel
	a.checksumMu.Unlock()
	return ctx, cancel
}

// playsetChecksum does the work of PlaysetChecksum for a game with a known scheme.
func (a *App) playsetChecksum(ctx context.Context, cfg game.GameConfig, playsetName string) (PlaysetChecksumResult, error) {
	unavailable := func(format string, args ...any) (PlaysetChecksumResult, error) {
		return PlaysetChecksumResult{Status: checksumUnavailable, Reason: fmt.Sprintf(format, args...)}, nil
	}
	if strings.TrimSpace(playsetName) == "" {
		return unavailable("No playset is loaded.")
	}
	p, err := a.playsets.Load(ctx, cfg.ID, playsetName)
	if err != nil {
		return PlaysetChecksumResult{}, err
	}
	installDir, ok := a.resolveInstallDir(cfg)
	if !ok {
		return unavailable("%s's install folder was not found.", cfg.DisplayName)
	}
	userDir, err := cfg.UserDataDir()
	if err != nil {
		return PlaysetChecksumResult{}, err
	}
	modDir := filepath.Join(userDir, "mod")

	scanned, err := scan.Scan(ctx, scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(cfg.ID)})
	if err != nil {
		return PlaysetChecksumResult{}, err
	}
	return computePlaysetChecksum(ctx, cfg, installDir, modDir, p.ModIDs, scanned.Mods, &a.checksumCache)
}

// computePlaysetChecksum calculates the checksum of the game with the mods named by ids (in
// load order) laid over it. Anything that makes the result untrustworthy - a mod that is not
// installed, one whose folder is missing - is reported as unavailable rather than left out.
func computePlaysetChecksum(ctx context.Context, cfg game.GameConfig, installDir, modDir string, ids []string, scanned []mod.Mod, cache *checksum.ResultCache) (PlaysetChecksumResult, error) {
	byID := make(map[string]mod.Mod, len(scanned))
	for _, m := range scanned {
		byID[m.ID] = m
	}

	var mods []checksum.Mod
	var notInstalled []string
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		m, ok := byID[id]
		if !ok {
			notInstalled = append(notInstalled, id)
			continue
		}
		mods = append(mods, checksum.Mod{
			ID:           m.ID,
			Name:         m.Descriptor.Name,
			RegistryID:   path.Join("mod", m.ID+".mod"),
			Content:      contentForChecksum(m),
			ReplacePaths: m.Descriptor.ReplacePath,
			Dependencies: m.Descriptor.Dependencies,
		})
	}
	if len(notInstalled) > 0 {
		return PlaysetChecksumResult{
			Status: checksumUnavailable,
			Reason: fmt.Sprintf("%s in this playset %s not installed: %s.", countOf(len(notInstalled), "mod"), isAre(len(notInstalled)), strings.Join(notInstalled, ", ")),
		}, nil
	}

	res, err := checksum.Compute(ctx, checksum.Input{
		Algorithm:        checksum.Algorithm(cfg.ChecksumAlgorithm),
		GameDir:          installDir,
		LauncherSettings: filepath.Join(installDir, filepath.FromSlash(cfg.LauncherSettingsPath)),
		ModDir:           modDir,
		Mods:             mods,
		Cache:            cache,
	})
	var missing *checksum.MissingContentError
	switch {
	case errors.As(err, &missing):
		return PlaysetChecksumResult{
			Status: checksumUnavailable,
			Reason: fmt.Sprintf("%s in this playset %s no files on disk: %s.", countOf(len(missing.Mods), "mod"), hasHave(len(missing.Mods)), strings.Join(missing.Mods, ", ")),
		}, nil
	case err != nil:
		return PlaysetChecksumResult{}, err
	}
	return PlaysetChecksumResult{
		Status:   checksumReady,
		Checksum: res.Checksum,
		Files:    res.Files,
		Mods:     len(res.Order),
		Warnings: res.Warnings,
	}, nil
}

// contentForChecksum is where the game finds m's files. A mod whose folder was not found has
// none, which the calculation reports rather than skips.
func contentForChecksum(m mod.Mod) string {
	if m.ContentMissing {
		return ""
	}
	return m.ContentPath
}

func countOf(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func hasHave(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}
