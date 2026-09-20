package modupdates

import (
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// SourceWorkshop is the Source value of a Steam Workshop mod - the only kind
// the Workshop is asked about.
const SourceWorkshop = "workshop"

// ModInput is what Build needs to know about one scanned mod.
type ModInput struct {
	ID           string
	Name         string
	Source       string
	Version      string
	RemoteFileID string
	Content      Fingerprint
}

// Workshop is what asking Steam about the game's Workshop mods gave.
type Workshop struct {
	// OK is false when Steam could not be reached: nothing in Details is then
	// trusted, and Build keeps what the previous snapshot knew.
	OK      bool
	Details map[string]steamapi.PublishedFileDetails
	// PageLive says, for items the API answered "not found" (result other than
	// 1) for, whether the item's own Workshop page is there. The API alone does
	// not prove a deletion - it says "not found" for some items whose page is up
	// (see steamapi.ItemPageExists) - so an item only counts as deleted when its
	// page is confirmed missing. An id absent from this map was not (or could not
	// be) checked, which is "unknown", never "deleted".
	PageLive map[string]bool
}

// Build makes the snapshot for the mods just scanned. prev is the last one
// taken (or nil): what it knew about the Workshop is kept for a mod Steam had
// no answer for this time, and a mod that was already gone keeps the date it
// first went missing.
func Build(mods []ModInput, ws Workshop, prev *Snapshot, now time.Time) Snapshot {
	snap := Snapshot{TakenAt: now.Unix(), Mods: make(map[string]Record, len(mods))}
	for _, m := range mods {
		rec := Record{
			Name:         m.Name,
			Source:       m.Source,
			Version:      m.Version,
			RemoteFileID: m.RemoteFileID,
			Content:      m.Content,
		}
		var old Record
		hadOld := false
		if prev != nil {
			old, hadOld = prev.Mods[m.ID]
		}
		if m.Source == SourceWorkshop && m.RemoteFileID != "" {
			if d, ok := ws.Details[m.RemoteFileID]; ws.OK && ok {
				switch {
				case d.Banned:
					rec.WorkshopGone = true
				case d.Result != 1:
					if live, checked := ws.PageLive[m.RemoteFileID]; checked {
						rec.WorkshopGone = !live
					} else {
						// Not confirmed either way: keep what was known, and
						// never claim a deletion nothing confirmed.
						rec.WorkshopGone = hadOld && old.WorkshopGone
					}
				}
				rec.WorkshopUpdated = d.TimeUpdated
			} else if hadOld {
				rec.WorkshopGone = old.WorkshopGone
			}
			// A missing answer, or an item that no longer has a date, keeps
			// the last date known rather than resetting it.
			if rec.WorkshopUpdated == 0 && hadOld {
				rec.WorkshopUpdated = old.WorkshopUpdated
			}
			if rec.WorkshopGone {
				if hadOld && old.WorkshopGone && old.GoneSince > 0 {
					rec.GoneSince = old.GoneSince
				} else {
					rec.GoneSince = now.Unix()
				}
			}
		}
		snap.Mods[m.ID] = rec
	}
	return snap
}
