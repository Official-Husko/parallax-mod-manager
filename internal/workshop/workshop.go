// Package workshop publishes or updates a Steam Workshop item by driving
// companions/parallax-steam-helper - a separate Go module, built as its own
// standalone executable, never imported here - the same way
// internal/launchershim drives companions/launcher-shim. See the parent
// project's docs/workshop-upload.md for why this talks to the locally
// running Steam client at all, and companions/parallax-steam-helper/main.go
// for the helper's own side of the protocol documented here.
package workshop

import "context"

// PublishRequest is what a caller asks a Publisher to do. AppID and
// LibraryPath are resolved by the caller from the target game's own real,
// detected install directory (see internal/game.GameConfig.DetectInstall
// and internal/steam.FindGameInstallDir) - never guessed or searched for
// inside this package.
type PublishRequest struct {
	// AppID is the target game's own Steam AppID (e.g. 281990 for
	// Stellaris) - this is the AppID context steam_appid.txt puts the
	// helper process into, not this app's own.
	AppID uint32
	// LibraryPath is that game's own real libsteam_api.so/steam_api64.dll,
	// found inside its install directory.
	LibraryPath string
	// ItemID is 0 to publish a brand new Workshop item, or an existing
	// PublishedFileId to push an update to one already published.
	ItemID uint64

	Title         string
	Description   string
	ChangeNote    string
	ContentFolder string
	// PreviewFile is optional.
	PreviewFile string
	// Visibility is one of "private", "friendsOnly", "unlisted", "public" -
	// empty defaults to "private", the least-exposed choice, in both this
	// package and the helper itself (see the helper's own Request.Visibility
	// doc comment for why that default is never left to Valve's own
	// item-creation default instead).
	Visibility string
}

// PublishProgress is one step of an in-flight publish, reported as it
// happens - see Publisher.Publish's onProgress.
type PublishProgress struct {
	// Stage is one of: "opening", "initialized", "creating", "created",
	// "updating", "uploading", "done", "error".
	Stage string
	// Message is set on "error" - a short, human-readable explanation.
	Message string
	// Processed/Total mirror Steam's own live upload byte counts during
	// "uploading" - both 0 on every other stage.
	Processed uint64
	Total     uint64
	// PublishedFileID is set from "created" onward.
	PublishedFileID uint64
}

// PublishResult is a successful publish's own outcome.
type PublishResult struct {
	// PublishedFileID is the Workshop item's id - the same one already
	// reported via PublishProgress once known, repeated here so a caller
	// that only cares about the final outcome doesn't have to track
	// progress events itself.
	PublishedFileID uint64
}

// Publisher publishes or updates a Workshop item, reporting progress as it
// happens through onProgress (which may be nil). The only real
// implementation is HelperPublisher; the interface exists so callers (and
// their own tests) never depend on companions/parallax-steam-helper being
// present on disk.
type Publisher interface {
	Publish(ctx context.Context, req PublishRequest, onProgress func(PublishProgress)) (PublishResult, error)
}
