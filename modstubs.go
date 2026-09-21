package main

import (
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
)

// modStubsEvent tells the frontend what launching did to the game's mod stubs, so the user hears
// about a mod that was silently not loading (or still cannot load) instead of finding out in game.
const modStubsEvent = "mod-stubs-changed"

// ModStubsReport is the modStubsEvent payload: mod names, by what happened to them.
type ModStubsReport struct {
	GameID string
	// Fixed are mods whose stub pointed at a folder that no longer exists and now points at the
	// folder the mod was found in; Created are mods that had no stub at all and got one.
	Fixed   []string
	Created []string
	// Missing are enabled mods whose folder was not found anywhere, so nothing could be done.
	Missing []string
	// Failed are mods whose stub could not be written (a read-only file, a full disk).
	Failed []string
}

// ensureModStubs makes every enabled mod loadable by the game before a launch. The game finds a
// classic-format mod only through the stub in its own mod folder (dlc_load.json names the stub,
// the stub's path= names the folder), while this app finds mods by scanning - including in the
// user's extra mod folders and by reconnecting a stub whose folder moved. Left alone, a mod that
// this app lists as fine can be one the game never loads (see scan.EnsureStub).
//
// One mod that cannot be mended never stops the launch: it is logged, reported to the frontend
// and the rest carry on.
func (a *App) ensureModStubs(gameID, modDir string, enabled []string, scanned []mod.Mod) {
	log := applog.For("Launch")
	byID := make(map[string]mod.Mod, len(scanned))
	for _, m := range scanned {
		byID[m.ID] = m
	}

	report := ModStubsReport{GameID: gameID}
	for _, id := range enabled {
		m, ok := byID[id]
		if !ok {
			continue
		}
		name := m.Descriptor.Name
		if name == "" {
			name = m.ID
		}
		if m.ContentMissing {
			log.Warnf("'%s' is enabled but its folder was not found (its stub says %s), so the game will not load it", name, stubPathForLog(m, modDir))
			report.Missing = append(report.Missing, name)
			continue
		}
		change, changed, err := scan.EnsureStub(m, modDir)
		switch {
		case err != nil:
			log.Warnf("could not make '%s' loadable for the game: %v", name, err)
			report.Failed = append(report.Failed, name)
		case changed && change.Created:
			log.Infof("wrote %s for '%s', pointing at %s (the game had no stub to find it by)", filepath.Base(change.File), name, change.NewPath)
			report.Created = append(report.Created, name)
		case changed:
			log.Infof("repaired %s for '%s': it pointed at %s, which does not exist; it now points at %s", filepath.Base(change.File), name, pathForLog(change.OldPath), change.NewPath)
			report.Fixed = append(report.Fixed, name)
		}
	}

	if len(report.Fixed)+len(report.Created)+len(report.Missing)+len(report.Failed) > 0 {
		a.emit(modStubsEvent, report)
	}
}

// stubPathForLog is the path m's stub declares, for a log line.
func stubPathForLog(m mod.Mod, modDir string) string {
	if m.Descriptor.Path == "" {
		return "no path"
	}
	p := m.Descriptor.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(modDir, p)
	}
	return p
}

// pathForLog spells an empty path as what it means.
func pathForLog(p string) string {
	if strings.TrimSpace(p) == "" {
		return "nothing (no path line)"
	}
	return p
}
