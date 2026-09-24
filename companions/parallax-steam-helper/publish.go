package main

import (
	"fmt"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/companions/parallax-steam-helper/steamworks"
)

// apiCallTimeout bounds every single blocking Steamworks call this helper
// makes (CreateItem, SubmitItemUpdate) - long enough for a real upload over a
// slow connection, short enough that a genuinely stuck call is eventually
// reported rather than hanging this helper (and whatever is waiting on it)
// forever.
const apiCallTimeout = 2 * time.Minute

// steamClient is the subset of *steamworks.Client this file drives, as an
// interface so publish is testable against a fake (see publish_test.go)
// without ever dlopen-ing a real library.
type steamClient interface {
	Init() bool
	Shutdown()
	AppID() uint32
	PersonaName() string
	CreateItemAndWait(appID uint32, fileType steamworks.FileType, timeout time.Duration) (steamworks.CreateItemResult, error)
	StartItemUpdate(appID uint32, itemID uint64) uint64
	SetItemTitle(handle uint64, title string) bool
	SetItemDescription(handle uint64, description string) bool
	SetItemVisibility(handle uint64, visibility steamworks.Visibility) bool
	SetItemContent(handle uint64, contentFolder string) bool
	SetItemPreview(handle uint64, previewFile string) bool
	SubmitItemUpdateAndWait(handle uint64, changeNote string, timeout time.Duration, onProgress func(status steamworks.UpdateStatus, processed, total uint64)) (steamworks.SubmitItemUpdateResult, error)
}

// identity answers an "identity"-mode Request: just SteamAPI_Init, read the
// signed-in user's own persona name, and shut down again - no item is
// created, updated, or even looked up. Kept as its own small flow rather
// than a branch inside publish, since the two share only Init/Shutdown/AppID
// and nothing about items at all.
func identity(client steamClient, req Request, emit func(Event)) error {
	emit(Event{Stage: "opening"})
	if !client.Init() {
		return fmt.Errorf("SteamAPI_Init failed - is Steam running and logged in, with %s/steam_appid.txt in place?", req.WorkDir)
	}
	defer client.Shutdown()

	if gotAppID := client.AppID(); gotAppID != req.AppID {
		return fmt.Errorf("Steam reports AppID %d, expected %d - steam_appid.txt did not take effect", gotAppID, req.AppID)
	}
	emit(Event{Stage: "done", PersonaName: client.PersonaName()})
	return nil
}

// visibilityFromString maps Request.Visibility to steamworks.Visibility,
// defaulting an empty string to Private - deliberately the least-exposed
// choice, never assumed from whatever Valve's own item-creation default
// happens to be (see Request.Visibility's own doc comment).
func visibilityFromString(s string) (steamworks.Visibility, error) {
	switch s {
	case "", "private":
		return steamworks.VisibilityPrivate, nil
	case "friendsOnly":
		return steamworks.VisibilityFriendsOnly, nil
	case "unlisted":
		return steamworks.VisibilityUnlisted, nil
	case "public":
		return steamworks.VisibilityPublic, nil
	default:
		return 0, fmt.Errorf("unknown visibility %q", s)
	}
}

// publish drives one full create-or-update flow against client, emitting an
// Event for each meaningful step via emit. Never calls
// SteamAPI_RestartAppIfNecessary - this package's steamClient interface
// doesn't even expose it, and steamworks.Open never resolves it either.
func publish(client steamClient, req Request, emit func(Event)) error {
	visibility, err := visibilityFromString(req.Visibility)
	if err != nil {
		return err
	}

	emit(Event{Stage: "opening"})
	if !client.Init() {
		return fmt.Errorf("SteamAPI_Init failed - is Steam running and logged in, with %s/steam_appid.txt in place?", req.WorkDir)
	}
	defer client.Shutdown()

	if gotAppID := client.AppID(); gotAppID != req.AppID {
		return fmt.Errorf("Steam reports AppID %d, expected %d - steam_appid.txt did not take effect", gotAppID, req.AppID)
	}
	emit(Event{Stage: "initialized"})

	itemID := req.ItemID
	if itemID == 0 {
		emit(Event{Stage: "creating"})
		created, err := client.CreateItemAndWait(req.AppID, steamworks.FileTypeCommunity, apiCallTimeout)
		if err != nil {
			return err
		}
		if created.Result != steamworks.ResultOK {
			return fmt.Errorf("CreateItem did not succeed: Steam result %d", created.Result)
		}
		itemID = created.PublishedFileID
		emit(Event{Stage: "created", PublishedFileID: itemID})
	}

	handle := client.StartItemUpdate(req.AppID, itemID)
	if req.Title != "" {
		client.SetItemTitle(handle, req.Title)
	}
	if req.Description != "" {
		client.SetItemDescription(handle, req.Description)
	}
	client.SetItemVisibility(handle, visibility)
	if req.ContentFolder != "" {
		client.SetItemContent(handle, req.ContentFolder)
	}
	if req.PreviewFile != "" {
		client.SetItemPreview(handle, req.PreviewFile)
	}

	emit(Event{Stage: "updating", PublishedFileID: itemID})
	result, err := client.SubmitItemUpdateAndWait(handle, req.ChangeNote, apiCallTimeout, func(_ steamworks.UpdateStatus, processed, total uint64) {
		emit(Event{Stage: "uploading", PublishedFileID: itemID, Processed: processed, Total: total})
	})
	if err != nil {
		return err
	}
	if result.Result != steamworks.ResultOK {
		return fmt.Errorf("SubmitItemUpdate did not succeed: Steam result %d (item %d already exists - check its Workshop page directly)", result.Result, itemID)
	}

	emit(Event{Stage: "done", PublishedFileID: result.PublishedFileID})
	return nil
}
