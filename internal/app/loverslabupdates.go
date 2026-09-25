package app

import (
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabtracking"
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

	// modID/entry pairs actually worth a real fetch - an entry with no baseline is
	// filtered out up front, so it never takes one of the 4 concurrent slots below
	// for a fetch whose result would just be thrown away.
	type candidate struct {
		modID string
		entry loverslabtracking.Entry
	}
	var toCheck []candidate
	for modID, entry := range installs {
		if entry.InstalledDateModified == "" {
			// No real baseline was ever captured for this one (the detail fetch at
			// install time failed) - nothing to compare against, so this is
			// "unknown", never "updated". A later reinstall fixes it permanently.
			continue
		}
		toCheck = append(toCheck, candidate{modID, entry})
	}

	// Fetched with real concurrency (mirroring CheckModUpdates' own Workshop-
	// existence check - see modupdates.go), not one at a time: a person with two
	// dozen mods installed from LoversLab was otherwise looking at two dozen
	// sequential page fetches - several real seconds - every time this runs (on
	// startup, and every few hours), competing with whatever the person is doing
	// in Browse right then for the exact same site. 4 at once, the same limit
	// already chosen there, keeps this fast without hammering the site.
	var mu sync.Mutex
	var changes []modupdates.Change
	backfilled := false
	g, gctx := errgroup.WithContext(a.baseContext())
	g.SetLimit(4)
	for _, c := range toCheck {
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			detail, err := a.loverslab.getFileDetail(gctx, client, c.entry.FileURL)
			if err != nil {
				log.Warnf("checking '%s' for updates failed, skipped: %v", c.entry.Title, err)
				return nil
			}

			mu.Lock()
			defer mu.Unlock()
			if c.entry.ThumbnailURL == "" && len(detail.Screenshots) > 0 && detail.Screenshots[0].ThumbnailURL != "" {
				c.entry.ThumbnailURL = detail.Screenshots[0].ThumbnailURL
				installs[c.modID] = c.entry
				backfilled = true
			}
			if detail.DateModified == "" || detail.DateModified == c.entry.InstalledDateModified {
				return nil
			}
			changes = append(changes, modupdates.Change{
				ModID:  c.modID,
				Name:   c.entry.Title,
				Source: "loverslab",
				Kind:   modupdates.KindUpdated,
				New:    true,
			})
			return nil
		})
	}
	_ = g.Wait()

	if backfilled {
		if err := a.loverslabInstalls.Save(gameID, installs); err != nil {
			log.Warnf("could not save backfilled thumbnails for '%s': %v", a.gameLabel(gameID), err)
		}
	}
	if changes == nil {
		changes = []modupdates.Change{}
	}
	return changes, nil
}
