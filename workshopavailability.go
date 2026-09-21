package main

import (
	"fmt"
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// WorkshopAvailability is one Workshop item that is not an ordinary listed one:
// unlisted, private or deleted. Ordinary items, and ones nothing can be said about,
// are not returned.
type WorkshopAvailability struct {
	// RemoteFileID is the Workshop item's id, as on the mod's descriptor.
	RemoteFileID string
	// State is "unlisted", "private" or "deleted".
	State string
	// Reason says how that was worked out ("record", "friends", "denied", "banned",
	// "page-up", "page-gone" - see steamapi.Reason*), so the interface can say how
	// sure the app is.
	Reason string
}

// WorkshopAvailability says which of gameID's Workshop mods are unlisted, private
// or deleted, for the flags on the mod lists and the detail panel. It works from the
// Workshop details already fetched (see WorkshopDetails) and, for the few items
// Steam answered "not found" for, the item's own page (remembered for the session).
// Nothing is guessed: an item Steam could not be asked about, or whose page could
// not be read, is left out.
func (a *App) WorkshopAvailability(gameID string) ([]WorkshopAvailability, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	byID, err := a.workshopDetails.Get(a.ctx, cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		return nil, err
	}
	pages := a.confirmWorkshopPages(byID, false)
	result := classifyWorkshop(byID, pages)

	counts := map[string]int{}
	for _, r := range result {
		counts[r.State]++
	}
	if len(result) > 0 {
		applog.For("Steam").Infof("'%s' Workshop mods that are not ordinary listed ones: %d unlisted, %d private, %d deleted",
			cfg.DisplayName, counts[string(steamapi.AvailabilityUnlisted)], counts[string(steamapi.AvailabilityPrivate)], counts[string(steamapi.AvailabilityDeleted)])
	}
	return result, nil
}

// classifyWorkshop applies steamapi.Classify to every item, keeping the ones that
// are unlisted, private or deleted, in id order.
func classifyWorkshop(details map[string]steamapi.PublishedFileDetails, pageLive map[string]bool) []WorkshopAvailability {
	var out []WorkshopAvailability
	for id, d := range details {
		live, checked := pageLive[id]
		s := steamapi.Classify(d, live, checked)
		switch s.State {
		case steamapi.AvailabilityUnlisted, steamapi.AvailabilityPrivate, steamapi.AvailabilityDeleted:
			out = append(out, WorkshopAvailability{RemoteFileID: id, State: string(s.State), Reason: s.Reason})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RemoteFileID < out[j].RemoteFileID })
	return out
}
