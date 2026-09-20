package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/modupdates"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// CheckModUpdates says what happened to gameID's mods since the app last
// started: updated, changed on disk, removed, or deleted from the Steam
// Workshop (see internal/modupdates). It scans the mods, fingerprints their
// files, asks Steam about the Workshop ones and compares the result with what
// the previous run recorded, which it then replaces for the next run.
//
// The comparison point is fixed for the whole run, so calling this again (after
// the mod folder changed, or with refresh to see an update published since the
// app started) keeps reporting everything since that last startup.
// refresh asks Steam again instead of trusting what it said earlier this run.
//
// A Steam failure is not an error: the report says so and still covers the
// changes on disk.
func (a *App) CheckModUpdates(gameID string, refresh bool) (modupdates.Report, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return modupdates.Report{}, fmt.Errorf("app: unknown game %q", gameID)
	}
	log := applog.For("Updates")
	timer := log.Begin()

	scanResult, err := scan.Scan(a.ctx, scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		timer.Errorf("couldn't scan '%s' for updates: %v", cfg.DisplayName, err)
		return modupdates.Report{}, err
	}
	inputs := modUpdateInputs(a.ctx, scanResult.Mods)

	ws, wsError := a.workshopState(gameID, cfg, scanResult.Mods, refresh)
	if wsError != "" {
		log.Warnf("couldn't ask Steam about '%s' Workshop mods: %s", cfg.DisplayName, wsError)
	}

	cur := modupdates.Build(inputs, ws, a.modUpdates.Previous(gameID), time.Now())
	report, saveErr := a.modUpdates.Commit(gameID, cur, ws.OK, wsError)
	if saveErr != nil {
		log.Warnf("couldn't save the mod snapshot for '%s': %v", cfg.DisplayName, saveErr)
	}
	if report.Unreadable {
		log.Warnf("'%s': no mods found although some were before - skipped comparing so a missing drive isn't read as every mod removed", cfg.DisplayName)
	}

	counts := map[modupdates.Kind]int{}
	for _, c := range report.Changes {
		if c.New {
			counts[c.Kind]++
		}
	}
	timer.Infof("'%s': %d mods checked since the last start: %d updated, %d changed, %d removed, %d deleted from the Workshop",
		cfg.DisplayName, report.ModsChecked, counts[modupdates.KindUpdated], counts[modupdates.KindChanged], counts[modupdates.KindRemoved], counts[modupdates.KindDeleted])
	return report, nil
}

// MarkModUpdatesSeen makes what is installed now the point later checks compare
// against, so what was reported stops being reported, and returns the report as
// it now reads.
func (a *App) MarkModUpdatesSeen(gameID string) modupdates.Report {
	return a.modUpdates.MarkSeen(gameID)
}

// workshopState asks Steam about the Workshop mods among mods. With none, there
// is nothing to ask and nothing that can fail. Otherwise a failure is returned
// as text, with Workshop.OK false so Build keeps what the last snapshot knew.
func (a *App) workshopState(gameID string, cfg game.GameConfig, mods []mod.Mod, refresh bool) (modupdates.Workshop, string) {
	hasWorkshop := false
	for _, m := range mods {
		if m.Source == mod.SourceWorkshop && m.Descriptor.RemoteFileID != "" {
			hasWorkshop = true
			break
		}
	}
	if !hasWorkshop {
		return modupdates.Workshop{OK: true, Details: map[string]steamapi.PublishedFileDetails{}}, ""
	}
	opts := library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)}
	var (
		details map[string]steamapi.PublishedFileDetails
		err     error
	)
	if refresh {
		details, err = a.workshopDetails.GetFresh(a.ctx, cfg, opts)
	} else {
		details, err = a.workshopDetails.Get(a.ctx, cfg, opts)
	}
	if err != nil {
		return modupdates.Workshop{}, err.Error()
	}
	return modupdates.Workshop{OK: true, Details: details}, ""
}

// modUpdateInputs turns scanned mods into what modupdates.Build takes,
// fingerprinting each one's files concurrently (a walk of every mod folder, the
// same cost as sizing them). The generated patch is left out: it is rewritten
// each time it is regenerated, which is not a change to a mod the user has.
func modUpdateInputs(ctx context.Context, mods []mod.Mod) []modupdates.ModInput {
	inputs := make([]modupdates.ModInput, 0, len(mods))
	for _, m := range mods {
		if m.ID == library.PatchModID {
			continue
		}
		name := m.Descriptor.Name
		if name == "" {
			name = m.ID
		}
		inputs = append(inputs, modupdates.ModInput{
			ID:           m.ID,
			Name:         name,
			Source:       modUpdateSource(m.Source),
			Version:      m.Descriptor.Version,
			RemoteFileID: m.Descriptor.RemoteFileID,
		})
	}

	// A mod whose content can't be found (a Workshop download still under way, a
	// drive that isn't connected) keeps an unknown fingerprint.
	contentByID := make(map[string]string, len(mods))
	for _, m := range mods {
		if !m.ContentMissing {
			contentByID[m.ID] = m.ContentPath
		}
	}
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())
	for i := range inputs {
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			f := modupdates.FingerprintDir(contentByID[inputs[i].ID])
			mu.Lock()
			inputs[i].Content = f
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return inputs
}

func modUpdateSource(s mod.Source) string {
	switch s {
	case mod.SourceWorkshop:
		return modupdates.SourceWorkshop
	case mod.SourceParadoxLauncher:
		return "paradox-launcher"
	default:
		return "local"
	}
}

// modUpdatesDir is where the per-game snapshots live under the config folder.
func modUpdatesDir(configAppDir string) string {
	return filepath.Join(configAppDir, "mod_updates")
}
