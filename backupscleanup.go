package main

import (
	"errors"
	"fmt"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/backup"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
	"github.com/Official-Husko/parallax-mod-manager/internal/scan"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// Kinds of clean-up (Settings > Backup > Free up space).
const (
	// cleanupInstalled removes backups of mods whose files are still in the Workshop
	// folder and that are still available on the Workshop: the copy is redundant.
	cleanupInstalled = "installed"
	// cleanupUnavailable removes backups of mods that are deleted or private on the
	// Workshop, which may well be the only copies left. Not recommended.
	cleanupUnavailable = "unavailable"
	// cleanupSelected removes exactly the backups named.
	cleanupSelected = "selected"
)

// BackupCleanupPlan is what a clean-up would remove, shown before anything is deleted.
type BackupCleanupPlan struct {
	Kind    string
	Entries []backup.Entry
	// Bytes is the space they take.
	Bytes int64
}

// BackupDeleteResult says what a delete did.
type BackupDeleteResult struct {
	// Deleted is how many backups were removed, Bytes the space that freed, and Skipped
	// how many were left alone (being copied right now, or no longer candidates).
	Deleted int
	Bytes   int64
	Skipped int
}

// PlanBackupCleanup lists the backups of gameID that a clean-up of the given kind
// ("installed" or "unavailable") would delete, without deleting anything.
func (a *App) PlanBackupCleanup(gameID, kind string) (BackupCleanupPlan, error) {
	entries, err := a.cleanupCandidates(gameID, kind)
	if err != nil {
		return BackupCleanupPlan{}, err
	}
	plan := BackupCleanupPlan{Kind: kind, Entries: entries}
	if plan.Entries == nil {
		plan.Entries = []backup.Entry{}
	}
	for _, e := range entries {
		plan.Bytes += e.Size
	}
	return plan, nil
}

// cleanupCandidates works out which of gameID's backups a clean-up of that kind
// removes. Nothing is a candidate unless the app is sure: an "installed" backup needs
// the mod's files on disk and Steam confirming it is still available, an "unavailable"
// one needs the Workshop (or how the copy was made) saying it is deleted or private.
func (a *App) cleanupCandidates(gameID, kind string) ([]backup.Entry, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	a.backup.mu.Lock()
	root := a.backup.rootLocked()
	a.backup.mu.Unlock()
	entries := backup.List(root, gameID)

	if kind == cleanupSelected {
		return entries, nil
	}
	if kind != cleanupInstalled && kind != cleanupUnavailable {
		return nil, fmt.Errorf("app: unknown clean-up %q", kind)
	}

	details, detailsErr := a.workshopDetails.Get(a.baseContext(), cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	var pages map[string]bool
	if detailsErr == nil {
		pages = a.confirmWorkshopPages(details, false)
	}
	standing := func(id string) steamapi.Standing {
		d, ok := details[id]
		if !ok {
			return steamapi.Standing{}
		}
		live, checked := pages[id]
		return steamapi.Classify(d, live, checked)
	}

	var out []backup.Entry
	switch kind {
	case cleanupInstalled:
		if detailsErr != nil {
			return nil, fmt.Errorf("couldn't ask Steam whether these mods are still available, so nothing was selected: %w", detailsErr)
		}
		installed, err := a.installedWorkshopItems(gameID)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !installed[e.RemoteFileID] {
				continue
			}
			if st := standing(e.RemoteFileID).State; st == steamapi.AvailabilityPublic || st == steamapi.AvailabilityUnlisted {
				out = append(out, e)
			}
		}
	case cleanupUnavailable:
		for _, e := range entries {
			st := standing(e.RemoteFileID).State
			switch {
			case st == steamapi.AvailabilityDeleted || st == steamapi.AvailabilityPrivate:
				out = append(out, e)
			case st == steamapi.AvailabilityPublic || st == steamapi.AvailabilityUnlisted:
				// Available again: not unavailable, whatever it was backed up for.
			case e.Reason == backup.ReasonDeleted || e.Reason == backup.ReasonPrivate:
				// Not installed, so nothing to ask Steam about: it was copied because it was
				// deleted or private, and nothing says that changed.
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// installedWorkshopItems is the set of Workshop item ids whose files are on disk.
func (a *App) installedWorkshopItems(gameID string) (map[string]bool, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return nil, fmt.Errorf("app: unknown game %q", gameID)
	}
	res, err := scan.Scan(a.baseContext(), scan.Options{Game: cfg, SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)})
	if err != nil {
		return nil, err
	}
	installed := map[string]bool{}
	for _, m := range res.Mods {
		if m.Source == mod.SourceWorkshop && m.Descriptor.RemoteFileID != "" && !m.ContentMissing {
			installed[m.Descriptor.RemoteFileID] = true
		}
	}
	return installed, nil
}

// DeleteBackups removes backups of gameID. For "installed" and "unavailable" the
// clean-up is worked out again first and only the ids in itemIDs that are still
// candidates are removed (what was shown may be out of date); "selected" removes the
// named ones that exist. A backup being written right now is left alone.
func (a *App) DeleteBackups(gameID, kind string, itemIDs []string) (BackupDeleteResult, error) {
	candidates, err := a.cleanupCandidates(gameID, kind)
	if err != nil {
		return BackupDeleteResult{}, err
	}
	allowed := make(map[string]bool, len(candidates))
	for _, e := range candidates {
		allowed[e.RemoteFileID] = true
	}

	b := &a.backup
	b.mu.Lock()
	root := b.rootLocked()
	var toDelete []string
	skipped := 0
	seen := map[string]bool{}
	for _, id := range itemIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if !allowed[id] || b.inflight[id] {
			skipped++
			continue
		}
		toDelete = append(toDelete, id)
	}
	b.mu.Unlock()
	if len(toDelete) == 0 {
		return BackupDeleteResult{Skipped: skipped}, nil
	}

	// A copy in progress checks the limits against what is there: let it finish first.
	b.copyMu.Lock()
	deleted, delErr := backup.Delete(root, gameID, toDelete)
	b.copyMu.Unlock()

	result := BackupDeleteResult{Deleted: len(deleted.IDs), Bytes: deleted.Bytes, Skipped: skipped + len(toDelete) - len(deleted.IDs)}
	if len(deleted.IDs) > 0 {
		applog.For("Backup").Infof("deleted %d backups (%s freed): %s", len(deleted.IDs), humanSize(deleted.Bytes), cleanupText(kind))
		a.emit("backups-changed")
		a.retryHeldBackups()
	}
	if delErr != nil {
		return result, errors.New("some backups could not be removed: " + delErr.Error())
	}
	return result, nil
}

func cleanupText(kind string) string {
	switch kind {
	case cleanupInstalled:
		return "backups of mods still installed and available"
	case cleanupUnavailable:
		return "backups of mods that are deleted or private"
	}
	return "chosen by hand"
}

// retryHeldBackups tries again what a limit had held back: room was just made.
func (a *App) retryHeldBackups() {
	b := &a.backup
	b.mu.Lock()
	mode := b.settings.Mode
	games := map[string][]WorkshopAvailability{}
	for id, s := range b.states {
		games[id] = s
	}
	b.limitState = ""
	b.blocked = map[string]string{}
	b.mu.Unlock()
	if mode == backup.ModeOff {
		return
	}
	for id, s := range games {
		a.preserveWorkshopMods(id, s)
	}
}
