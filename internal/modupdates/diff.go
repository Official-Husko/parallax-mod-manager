package modupdates

import (
	"sort"
	"strings"
)

// Kind says what happened to a mod.
type Kind string

const (
	// KindUpdated: a new version - its descriptor version changed, or its
	// Workshop page was updated.
	KindUpdated Kind = "updated"
	// KindChanged: its files changed without either of the above (a local mod
	// that was edited, a file replaced in place).
	KindChanged Kind = "changed"
	// KindRemoved: it is no longer installed.
	KindRemoved Kind = "removed"
	// KindDeleted: it is still installed, but Steam says its Workshop item was
	// deleted or banned - it will never be updated again.
	KindDeleted Kind = "deleted"
)

// Change is one thing that happened to one mod.
type Change struct {
	ModID        string
	Name         string
	Source       string
	RemoteFileID string
	Kind         Kind
	// New is true when it happened since the snapshot compared against. A mod
	// that was already deleted from the Workshop then is still listed, as a
	// standing problem, with New false.
	New bool
	// FromVersion and ToVersion are the descriptor versions before and after,
	// set when they differ.
	FromVersion string
	ToVersion   string
	// WorkshopUpdated is the Workshop page's new update time (unix seconds),
	// set when the Workshop reported an update since the snapshot.
	WorkshopUpdated int64
	// FilesChanged is true when the mod's files differ from the snapshot;
	// FilesDelta and SizeDelta say by how much (both can be zero for a file
	// rewritten in place).
	FilesChanged bool
	FilesDelta   int
	SizeDelta    int64
	// GoneSince is when the mod was first seen deleted (KindDeleted only).
	GoneSince int64
}

// kindOrder is the order changes are listed in: what most needs a look first.
var kindOrder = map[Kind]int{KindDeleted: 0, KindRemoved: 1, KindUpdated: 2, KindChanged: 3}

// Diff lists what happened to the mods in cur since base. With no base (the
// first run, or no readable earlier snapshot) nothing has a "before", so only
// the mods already deleted from the Workshop are listed, as standing entries.
// A mod that was not in base is not reported: installing something is not a
// change to it.
func Diff(base *Snapshot, cur Snapshot) []Change {
	var changes []Change
	for id, c := range cur.Mods {
		var b Record
		had := false
		if base != nil {
			b, had = base.Mods[id]
		}
		if c.WorkshopGone {
			changes = append(changes, Change{
				ModID: id, Name: c.Name, Source: c.Source, RemoteFileID: c.RemoteFileID,
				Kind: KindDeleted, New: had && !b.WorkshopGone, GoneSince: c.GoneSince,
			})
			continue
		}
		if !had {
			continue
		}
		change := Change{ModID: id, Name: c.Name, Source: c.Source, RemoteFileID: c.RemoteFileID}
		versionChanged := b.Version != c.Version && (b.Version != "" || c.Version != "")
		workshopUpdated := b.WorkshopUpdated > 0 && c.WorkshopUpdated > b.WorkshopUpdated
		if b.Content.Known && c.Content.Known && b.Content != c.Content {
			change.FilesChanged = true
			change.FilesDelta = c.Content.Files - b.Content.Files
			change.SizeDelta = c.Content.Size - b.Content.Size
		}
		switch {
		case versionChanged || workshopUpdated:
			change.Kind = KindUpdated
			if versionChanged {
				change.FromVersion, change.ToVersion = b.Version, c.Version
			}
			if workshopUpdated {
				change.WorkshopUpdated = c.WorkshopUpdated
			}
		case change.FilesChanged:
			change.Kind = KindChanged
		default:
			continue
		}
		change.New = true
		changes = append(changes, change)
	}
	if base != nil {
		for id, b := range base.Mods {
			if _, still := cur.Mods[id]; !still {
				changes = append(changes, Change{
					ModID: id, Name: b.Name, Source: b.Source, RemoteFileID: b.RemoteFileID,
					Kind: KindRemoved, New: true,
				})
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		if kindOrder[a.Kind] != kindOrder[b.Kind] {
			return kindOrder[a.Kind] < kindOrder[b.Kind]
		}
		if an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name); an != bn {
			return an < bn
		}
		return a.ModID < b.ModID
	})
	return changes
}
