// Command parallax-steam-helper talks to the locally-running Steam client on
// behalf of Parallax Mod Manager, to publish or update a Workshop item for
// whichever Paradox game the caller names - isolated in its own process, the
// same reason companions/launcher-shim is its own separate binary rather than
// code inside the main Wails process. Reads one JSON Request from stdin,
// writes newline-delimited JSON Events to stdout as it progresses, and exits
// 0 on a final "done" event or 1 on "error".
//
// Never calls SteamAPI_RestartAppIfNecessary - see steamworks.requiredSymbols,
// which never even resolves it - since that call launches the real game
// instead of letting this helper keep running as itself.
package main

// Request is the one JSON object this helper reads from stdin before doing
// anything else. internal/workshop, in the main app's own module, builds and
// writes this - mirrored there by hand, not shared, since these are two
// separate Go modules (the same reason companions/launcher-shim's own status
// type is mirrored by internal/launchershim rather than imported).
type Request struct {
	// LibraryPath is the target game's own real libsteam_api.so/
	// steam_api64.dll, already resolved by the caller from that game's own
	// install directory - never guessed or searched for here.
	LibraryPath string `json:"libraryPath"`
	// WorkDir is a fresh, dedicated directory this helper writes its own
	// steam_appid.txt into and runs from - never the game's own install
	// directory, so nothing here ever touches a real game file.
	WorkDir string `json:"workDir"`
	AppID   uint32 `json:"appId"`

	// ItemID is 0 to publish a brand new Workshop item, or an existing
	// PublishedFileId to push an update to one already published.
	ItemID uint64 `json:"itemId"`

	Title         string `json:"title"`
	Description   string `json:"description"`
	ChangeNote    string `json:"changeNote"`
	ContentFolder string `json:"contentFolder"`
	// PreviewFile is optional - an item can be created and updated with no
	// preview image at all.
	PreviewFile string `json:"previewFile,omitempty"`
	// Visibility is one of "private", "friendsOnly", "unlisted", "public" -
	// empty defaults to "private" (see visibilityFromString), deliberately
	// the least-exposed option rather than Valve's own item-creation
	// default, which this project does not rely on being any particular
	// value (see the parent project's docs/workshop-upload.md).
	Visibility string `json:"visibility"`
}

// Event is one line this helper writes to stdout - newline-delimited JSON,
// so internal/workshop can stream progress back to the frontend as each
// line arrives, rather than waiting for the whole process to exit.
type Event struct {
	// Stage is one of: "opening", "initialized", "creating", "created",
	// "updating", "uploading", "done", "error".
	Stage string `json:"stage"`
	// Message is set on "error" - a short, human-readable explanation,
	// never a raw Go error's full Errorf chain.
	Message string `json:"message,omitempty"`
	// Processed/Total mirror Steam's own live upload byte counts during
	// "uploading" (see steamworks.Client.SubmitItemUpdateAndWait) - both 0
	// on every other stage.
	Processed uint64 `json:"processed,omitempty"`
	Total     uint64 `json:"total,omitempty"`
	// PublishedFileID is set from "created" onward, and again, unchanged,
	// on the final "done" event.
	PublishedFileID uint64 `json:"publishedFileId,omitempty"`
}
