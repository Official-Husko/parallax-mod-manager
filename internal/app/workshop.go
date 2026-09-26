package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/launch"
	"github.com/Official-Husko/parallax-mod-manager/internal/library"
	"github.com/Official-Husko/parallax-mod-manager/internal/modedit"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
	"github.com/Official-Husko/parallax-mod-manager/internal/toolmark"
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
// requestID (any string the caller makes up per attempt) tags this run so
// CancelPublish can stop it - see that function's own doc comment for
// exactly what "cancel" can and can't mean here.
func (a *App) PublishModToWorkshop(gameID, modID, requestID string, req WorkshopPublishRequest) (WorkshopPublishResult, error) {
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

	// Written before the real upload, deliberately - see internal/toolmark's own doc
	// comment - so it's included in what actually gets published, not added after the
	// fact. Opt-in (Settings > Advanced, offered once on a person's first-ever
	// publish - see the Publish tab) and never the reason a publish itself fails.
	a.preferencesMu.Lock()
	shareToolMark := a.preferences.ShareToolMark
	a.preferencesMu.Unlock()
	if shareToolMark {
		if err := toolmark.Record(contentFolder, "Published to Steam Workshop"); err != nil {
			applog.For("Workshop").Warnf("noting Parallax Mod Manager in '%s' failed: %v", modID, err)
		}
	}

	ctx, cancel := context.WithCancel(a.baseContext())
	a.workshopMu.Lock()
	if a.workshopCancel == nil {
		a.workshopCancel = map[string]context.CancelFunc{}
	}
	a.workshopCancel[requestID] = cancel
	a.workshopMu.Unlock()
	defer func() {
		a.workshopMu.Lock()
		delete(a.workshopCancel, requestID)
		a.workshopMu.Unlock()
		cancel()
	}()

	result, err := a.workshopPublisherOrDefault().Publish(ctx, pubReq, func(p workshop.PublishProgress) {
		a.emit("workshop-publish-progress", gameID, WorkshopPublishProgress{
			Stage:           p.Stage,
			Message:         p.Message,
			Processed:       p.Processed,
			Total:           p.Total,
			PublishedFileID: formatFileID(p.PublishedFileID),
		})
	})
	if err != nil {
		if ctx.Err() != nil {
			return WorkshopPublishResult{}, fmt.Errorf("publishing was cancelled - Steam may have been left with a partially updated item; check its Workshop page")
		}
		return WorkshopPublishResult{}, err
	}
	return WorkshopPublishResult{PublishedFileID: formatFileID(result.PublishedFileID)}, nil
}

// CancelPublish stops the PublishModToWorkshop call tagged with requestID, if it is still
// running - a no-op otherwise (it may already be finished). Steamworks' own flat API exposes no
// real "abort upload" call, so this only ever kills the companion process itself (the context
// cancellation reaches its exec.CommandContext, in internal/workshop) - never a clean mid-upload
// stop. Cancelling after the byte upload has actually started can leave the Workshop item
// partially updated; the frontend warns about this before calling here mid-upload, but this
// function itself does not gate on which stage is currently in flight.
func (a *App) CancelPublish(requestID string) {
	a.workshopMu.Lock()
	cancel := a.workshopCancel[requestID]
	a.workshopMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// SteamAccountInfo is who is currently signed into the local Steam client,
// for the Publish tab's own STEAM ACCOUNT card.
type SteamAccountInfo struct {
	PersonaName string
}

// SteamAccountInfo asks the local Steam client who is signed in, using
// gameID's own Steamworks library (any registered game with a real,
// findable install works equally well here - the signed-in identity is not
// really per-game, but SteamAPI_Init still needs a real AppID context to
// succeed at all).
func (a *App) SteamAccountInfo(gameID string) (SteamAccountInfo, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return SteamAccountInfo{}, fmt.Errorf("unknown game %q", gameID)
	}
	if cfg.SteamAppID == "" {
		return SteamAccountInfo{}, fmt.Errorf("%s has no Steam AppID configured", cfg.DisplayName)
	}
	appID64, err := strconv.ParseUint(cfg.SteamAppID, 10, 32)
	if err != nil {
		return SteamAccountInfo{}, fmt.Errorf("%s's Steam AppID %q is not numeric: %w", cfg.DisplayName, cfg.SteamAppID, err)
	}

	installDir, ok := a.resolveInstallDir(cfg)
	if !ok {
		return SteamAccountInfo{}, fmt.Errorf("%s's install could not be found - set its install path in Settings first", cfg.DisplayName)
	}
	libPath, err := steamLibraryPath(installDir)
	if err != nil {
		return SteamAccountInfo{}, err
	}

	result, err := a.workshopPublisherOrDefault().Identity(a.baseContext(), workshop.IdentityRequest{AppID: uint32(appID64), LibraryPath: libPath})
	if err != nil {
		return SteamAccountInfo{}, err
	}
	return SteamAccountInfo{PersonaName: result.PersonaName}, nil
}

// FilePreview is a small, resized preview of one file in a mod's own folder - the Publish tab's
// own file tree wants this for a picture the person might exclude, without opening it in an
// external viewer. Kind is "image" for a recognized picture (see modedit.HasPictureExtension),
// "none" for anything else - the frontend shows a generic icon then, never an error, since most
// files in a real mod are not pictures at all.
type FilePreview struct {
	Kind          string
	DataURI       string
	Width, Height int
	Bytes         int64
}

// PreviewModFile resolves relPath (forward-slashed, relative to modID's own content folder - the
// same convention library.FileEntry.RelPath/WorkshopPublishRequest.ExcludePaths already use, so
// the frontend only ever passes back a path it got from ListModFiles, never one it made up) and,
// for a recognized picture, returns a resized preview via the exact same modedit.PrepareThumbnail
// path the Edit tab's own thumbnail preview already uses - one image pipeline, not two.
func (a *App) PreviewModFile(gameID, modID, relPath string) (FilePreview, error) {
	cfg, ok := a.registry.Get(gameID)
	if !ok {
		return FilePreview{}, fmt.Errorf("unknown game %q", gameID)
	}
	contentFolder, err := library.ModFolderPath(a.baseContext(), cfg, library.Options{SteamRoots: a.steamRoots, ExtraFolders: a.extraModFolders(gameID)}, modID)
	if err != nil {
		return FilePreview{}, fmt.Errorf("finding %s's own content folder: %w", modID, err)
	}
	if !modedit.HasPictureExtension(relPath) {
		return FilePreview{Kind: "none"}, nil
	}

	full := filepath.Clean(filepath.Join(contentFolder, filepath.FromSlash(relPath)))
	root := filepath.Clean(contentFolder)
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return FilePreview{}, fmt.Errorf("app: %q is outside the mod's own folder", relPath)
	}

	th, err := modedit.PrepareThumbnail(full)
	if err != nil {
		return FilePreview{}, err
	}
	return FilePreview{
		Kind: "image", DataURI: "data:image/png;base64," + base64.StdEncoding.EncodeToString(th.PNG),
		Width: th.Width, Height: th.Height, Bytes: int64(len(th.PNG)),
	}, nil
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

// workshopOpenSequence returns, in the order OpenWorkshopPage should try
// them, the URL(s) for publishedFileID given mode (a person's raw
// WorkshopOpenMode preference string, straight from
// Preferences.WorkshopOpenMode). "browser" chosen explicitly is a single
// URL - the item's real public page, nothing else to fall back to. Every
// other value ("app", the default, or an empty or unrecognized one - e.g. a
// settings file saved before this preference existed, or while "app" was
// still the implicit zero value rather than a real default) tries the
// local Steam client first, with the public page as a second entry
// OpenWorkshopPage only reaches if opening Steam itself failed (not
// installed, no steam:// handler registered, and so on).
func workshopOpenSequence(mode, publishedFileID string) []string {
	if steamapi.WorkshopOpenMode(mode) == steamapi.WorkshopOpenModeBrowser {
		return []string{steamapi.WorkshopPageURL(publishedFileID)}
	}
	return []string{steamapi.WorkshopClientURL(publishedFileID), steamapi.WorkshopPageURL(publishedFileID)}
}

// OpenWorkshopPage opens a Workshop item's page using the person's own
// "Open in Workshop" preference (Settings > Steam API): the local Steam
// client by default, via its own documented steam://url/CommunityFilePage/
// protocol handler, or their default browser showing the item's real
// public page - either chosen explicitly, or reached automatically here if
// opening Steam itself didn't work. Only the final attempt's own error (if
// every URL failed) is returned.
func (a *App) OpenWorkshopPage(publishedFileID string) error {
	a.preferencesMu.Lock()
	mode := a.preferences.WorkshopOpenMode
	a.preferencesMu.Unlock()
	urls := workshopOpenSequence(mode, publishedFileID)
	launcher := launch.OSLauncher{}
	var err error
	for i, url := range urls {
		if err = launcher.OpenURL(url); err == nil {
			return nil
		}
		if i < len(urls)-1 {
			applog.For("Workshop").Warnf("opening %q for item %s failed (%v) - falling back to the browser", url, publishedFileID, err)
		}
	}
	return err
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
