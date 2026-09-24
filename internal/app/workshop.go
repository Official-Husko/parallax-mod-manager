package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/workshop"
)

// WorkshopPublishRequest is what the frontend asks PublishModToWorkshop to
// do. Title, Description and ItemID come straight from the mod's own
// already-loaded library.ModSummary (Name, ShortDescription, RemoteFileID) -
// the frontend already has these, so there is no reason to make it re-enter
// or re-fetch them. ContentFolder is deliberately not a field here at all:
// it is a real filesystem path, resolved server-side from gameID/modID (see
// PublishModToWorkshop), the same way OpenModFolder never accepts one from
// the frontend either.
type WorkshopPublishRequest struct {
	// ItemID is "" to publish a brand new Workshop item, or an existing
	// PublishedFileId (as a string - JavaScript numbers cannot carry a full
	// uint64 without precision loss) to push an update to one already
	// published - a mod's own RemoteFileID, unchanged.
	ItemID      string
	Title       string
	Description string
	ChangeNote  string
	// Visibility is one of "private", "friendsOnly", "unlisted", "public" -
	// empty defaults to "private" (see workshop.PublishRequest.Visibility).
	Visibility string
	// ExcludePaths lists files or folders, relative to the mod's own
	// content folder (forward-slashed, matching library.FileEntry.RelPath -
	// see ListModFiles), to leave out of the upload entirely. Excluding a
	// folder excludes everything under it. Never a filesystem path - the
	// frontend picks these from the same file list ListModFiles already
	// gives it, never anything it resolved on its own.
	ExcludePaths []string
}

// WorkshopPublishResult is a successful publish's own outcome.
type WorkshopPublishResult struct {
	// PublishedFileID is the Workshop item's id, as a string for the same
	// reason WorkshopPublishRequest.ItemID is.
	PublishedFileID string
}

// WorkshopPublishProgress is one step of an in-flight publish, sent to the
// frontend as it happens - see the "workshop-publish-progress" event
// PublishModToWorkshop emits.
type WorkshopPublishProgress struct {
	Stage           string
	Message         string
	Processed       uint64
	Total           uint64
	PublishedFileID string
}

// workshopPublisher returns a.workshopPublisher if a test has set one,
// otherwise the real workshop.HelperPublisher - the same nil-defaults-to-real
// seam a.emit uses for eventSink.
func (a *App) workshopPublisherOrDefault() workshop.Publisher {
	if a.workshopPublisher != nil {
		return a.workshopPublisher
	}
	return workshop.HelperPublisher{}
}

// PublishModToWorkshop publishes gameID's modID as a new Workshop item, or
// updates an existing one if req.ItemID is set, by driving
// companions/parallax-steam-helper against that game's own real, locally
// installed Steamworks library - see internal/workshop and the parent
// project's docs/workshop-upload.md for the whole mechanism and its real,
// confirmed limits. Progress streams to the frontend as
// "workshop-publish-progress" events (gameID, WorkshopPublishProgress) while
// this call is in flight; the return value is only the final outcome.
func (a *App) PublishModToWorkshop(gameID, modID string, req WorkshopPublishRequest) (WorkshopPublishResult, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return WorkshopPublishResult{}, fmt.Errorf("unknown game %q", gameID)
	}
	if cfg.SteamAppID == "" {
		return WorkshopPublishResult{}, fmt.Errorf("%s has no Steam AppID configured", cfg.DisplayName)
	}
	appID64, err := strconv.ParseUint(cfg.SteamAppID, 10, 32)
	if err != nil {
		return WorkshopPublishResult{}, fmt.Errorf("%s's Steam AppID %q is not numeric: %w", cfg.DisplayName, cfg.SteamAppID, err)
	}
	appID := uint32(appID64)

	installDir, ok := a.resolveInstallDir(cfg)
	if !ok {
		return WorkshopPublishResult{}, fmt.Errorf("%s's install could not be found - set its install path in Settings first", cfg.DisplayName)
	}
	libPath, err := steamLibraryPath(installDir)
	if err != nil {
		return WorkshopPublishResult{}, err
	}

	contentFolder, err := library.ModFolderPath(a.baseContext(), cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)}, modID)
	if err != nil {
		return WorkshopPublishResult{}, fmt.Errorf("finding %s's own content folder: %w", modID, err)
	}

	var itemID uint64
	if req.ItemID != "" {
		itemID, err = strconv.ParseUint(req.ItemID, 10, 64)
		if err != nil {
			return WorkshopPublishResult{}, fmt.Errorf("invalid item id %q: %w", req.ItemID, err)
		}
	}

	pubReq := workshop.PublishRequest{
		AppID:         appID,
		LibraryPath:   libPath,
		ItemID:        itemID,
		Title:         req.Title,
		Description:   req.Description,
		ChangeNote:    req.ChangeNote,
		ContentFolder: contentFolder,
		Visibility:    req.Visibility,
		ExcludePaths:  req.ExcludePaths,
	}

	result, err := a.workshopPublisherOrDefault().Publish(a.baseContext(), pubReq, func(p workshop.PublishProgress) {
		a.emit("workshop-publish-progress", gameID, WorkshopPublishProgress{
			Stage:           p.Stage,
			Message:         p.Message,
			Processed:       p.Processed,
			Total:           p.Total,
			PublishedFileID: formatFileID(p.PublishedFileID),
		})
	})
	if err != nil {
		return WorkshopPublishResult{}, err
	}
	return WorkshopPublishResult{PublishedFileID: formatFileID(result.PublishedFileID)}, nil
}

// formatFileID renders a PublishedFileId as a string, empty for 0 (not yet
// known) rather than the literal string "0", so the frontend can tell
// "nothing yet" apart from a real id at a glance.
func formatFileID(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

// steamLibraryPath finds installDir's own real Steamworks client library
// for this app's own host platform - companions/parallax-steam-helper can
// only dlopen a library matching its own OS (it is built once per this
// app's own host OS, per the root build.sh, same as every other companion),
// so a game installed for a different platform (a Windows-only Proton
// install browsed to from a Linux host, for instance) genuinely cannot be
// used for publishing from here - reported plainly rather than attempting a
// load that could never succeed.
func steamLibraryPath(installDir string) (string, error) {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{"steam_api64.dll", "steam_api.dll"}
	case "darwin":
		candidates = []string{"libsteam_api.dylib"}
	default:
		candidates = []string{"libsteam_api.so"}
	}
	for _, name := range candidates {
		p := filepath.Join(installDir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Steamworks library (%s) found in %s - Workshop publishing needs a native %s install of this game", strings.Join(candidates, " or "), installDir, runtime.GOOS)
}
