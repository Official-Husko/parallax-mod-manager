package app

import (
	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/modupdates"
)

// CheckLoversLabUpdates re-fetches each of gameID's mods that were installed from
// LoversLab (see internal/loverslabtracking) and reports the ones whose "Updated"
// timestamp has moved on since it was installed - a mirror of CheckModUpdates
// (modupdates.go), just simpler: LoversLab has no "banned"/"deleted" concept to track,
// and there is no separate "mark seen" action, since the natural way to resolve a
// LoversLab update is to actually install it (which is what advances
// InstalledDateModified in the first place - see LoversLabInstall). Reuses
// modupdates.Change/Kind directly (Source "loverslab") rather than a parallel report
// type, so the frontend can show these in the exact same list Workshop updates
// already appear in without either side needing to know about the other's mechanism.
//
// A tracked mod whose page could no longer be reached (removed from the site,
// a network hiccup) is skipped rather than reported as anything - "unknown" is not
// "updated", and this never touches what's actually installed.
func (a *App) CheckLoversLabUpdates(gameID string) ([]modupdates.Change, error) {
	log := applog.For("LoversLab")

	client, err := a.ensureLoversLabSession(a.baseContext())
	if err != nil {
		return nil, err
	}

	installs, err := a.loverslabInstalls.Load(gameID)
	if err != nil {
		return nil, err
	}

	// Backfilled opportunistically below, from the very same detail fetch this
	// function already does for its own reason (comparing DateModified) - never a
	// fetch run just for this. Lets a mod installed before ThumbnailURL existed pick
	// one up the next time this runs (on startup, and every few hours - see
	// features.md), instead of needing to be reinstalled to get a real card image.
	backfilled := false

	changes := make([]modupdates.Change, 0, len(installs))
	for modID, entry := range installs {
		if entry.InstalledDateModified == "" {
			// No real baseline was ever captured for this one (the detail fetch at
			// install time failed) - nothing to compare against, so this is
			// "unknown", never "updated". A later reinstall fixes it permanently.
			continue
		}
		detail, err := a.loverslab.getFileDetail(a.baseContext(), client, entry.FileURL)
		if err != nil {
			log.Warnf("checking '%s' for updates failed, skipped: %v", entry.Title, err)
			continue
		}
		if entry.ThumbnailURL == "" && len(detail.Screenshots) > 0 && detail.Screenshots[0].ThumbnailURL != "" {
			entry.ThumbnailURL = detail.Screenshots[0].ThumbnailURL
			installs[modID] = entry
			backfilled = true
		}
		if detail.DateModified == "" || detail.DateModified == entry.InstalledDateModified {
			continue
		}
		changes = append(changes, modupdates.Change{
			ModID:  modID,
			Name:   entry.Title,
			Source: "loverslab",
			Kind:   modupdates.KindUpdated,
			New:    true,
		})
	}
	if backfilled {
		if err := a.loverslabInstalls.Save(gameID, installs); err != nil {
			log.Warnf("could not save backfilled thumbnails for '%s': %v", a.gameLabel(gameID), err)
		}
	}
	return changes, nil
}
