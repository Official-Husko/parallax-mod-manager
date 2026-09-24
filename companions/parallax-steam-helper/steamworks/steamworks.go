package steamworks

import (
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// requiredSymbols are every un-versioned flat-API symbol this package calls,
// besides the two versioned interface accessors (resolved separately - see
// versions.go). Checked with Dlsym up front in Open, before any is registered
// with purego.RegisterLibFunc - that function has no error return of its own
// and resolves a missing symbol by panicking, which would otherwise crash this
// whole helper instead of reporting a clean, catchable error back to the
// caller (and, through it, to the person who asked to publish something).
var requiredSymbols = []string{
	"SteamAPI_Init",
	"SteamAPI_Shutdown",
	"SteamAPI_RunCallbacks",
	"SteamAPI_ISteamUtils_GetAppID",
	"SteamAPI_ISteamUtils_IsAPICallCompleted",
	"SteamAPI_ISteamUtils_GetAPICallResult",
	"SteamAPI_ISteamUGC_CreateItem",
	"SteamAPI_ISteamUGC_StartItemUpdate",
	"SteamAPI_ISteamUGC_SetItemTitle",
	"SteamAPI_ISteamUGC_SetItemDescription",
	"SteamAPI_ISteamUGC_SetItemVisibility",
	"SteamAPI_ISteamUGC_SetItemContent",
	"SteamAPI_ISteamUGC_SetItemPreview",
	"SteamAPI_ISteamUGC_SubmitItemUpdate",
	"SteamAPI_ISteamUGC_GetItemUpdateProgress",
	"SteamAPI_ISteamFriends_GetPersonaName",
}

// Client is a single open handle to a real, on-disk libsteam_api.so/
// steam_api64.dll, resolved and bound fresh by Open - never a global or
// package-level handle, so a caller decides exactly which game's own library
// gets loaded, and nothing here is left around for a second, unrelated call
// to accidentally reuse.
type Client struct {
	handle  uintptr
	utils   uintptr // ISteamUtils* - only valid once Init has succeeded
	ugc     uintptr // ISteamUGC* - only valid once Init has succeeded
	friends uintptr // ISteamFriends* - only valid once Init has succeeded

	fn steamFuncs
}

// steamFuncs holds every raw function pointer Open resolves through purego.
// Kept as its own struct, rather than loose Client fields, purely to keep
// Open's own registration block scannable as one list.
type steamFuncs struct {
	init         func() bool
	shutdown     func()
	runCallbacks func()

	getUtils   func() uintptr
	getUGC     func() uintptr
	getFriends func() uintptr

	getAppID           func(self uintptr) uint32
	isAPICallCompleted func(self uintptr, call uint64, pbFailed *bool) bool
	getAPICallResult   func(self uintptr, call uint64, pCallback unsafe.Pointer, cubCallback int32, iCallbackExpected int32, pbFailed *bool) bool
	getPersonaName     func(self uintptr) string

	createItem            func(self uintptr, appID uint32, fileType int32) uint64
	startItemUpdate       func(self uintptr, appID uint32, itemID uint64) uint64
	setItemTitle          func(self uintptr, handle uint64, title string) bool
	setItemDescription    func(self uintptr, handle uint64, description string) bool
	setItemVisibility     func(self uintptr, handle uint64, visibility int32) bool
	setItemContent        func(self uintptr, handle uint64, contentFolder string) bool
	setItemPreview        func(self uintptr, handle uint64, previewFile string) bool
	submitItemUpdate      func(self uintptr, handle uint64, changeNote string) uint64
	getItemUpdateProgress func(self uintptr, handle uint64, processed, total *uint64) int32
}

// Open dlopen's libPath - a real, already-installed game's own Steamworks
// library, never anything this project ships itself - and resolves every
// function this package calls through it, probing interface versions rather
// than assuming one (see versions.go). It does not call SteamAPI_Init and
// never calls SteamAPI_RestartAppIfNecessary at all (that would launch the
// real game); see (*Client).Init.
func Open(libPath string) (*Client, error) {
	handle, err := openLibrary(libPath)
	if err != nil {
		return nil, fmt.Errorf("steamworks: opening %s: %w", libPath, err)
	}

	resolves := func(name string) bool {
		return symbolExists(handle, name)
	}

	utilsAccessor, ok := resolveVersion(steamUtilsVersions, resolves)
	if !ok {
		return nil, fmt.Errorf("steamworks: %s exports no known ISteamUtils accessor (tried %v)", libPath, steamUtilsVersions)
	}
	ugcAccessor, ok := resolveVersion(steamUGCVersions, resolves)
	if !ok {
		return nil, fmt.Errorf("steamworks: %s exports no known ISteamUGC accessor (tried %v)", libPath, steamUGCVersions)
	}
	friendsAccessor, ok := resolveVersion(steamFriendsVersions, resolves)
	if !ok {
		return nil, fmt.Errorf("steamworks: %s exports no known ISteamFriends accessor (tried %v)", libPath, steamFriendsVersions)
	}
	for _, name := range requiredSymbols {
		if !resolves(name) {
			return nil, fmt.Errorf("steamworks: %s is missing required symbol %s", libPath, name)
		}
	}

	c := &Client{handle: handle}
	purego.RegisterLibFunc(&c.fn.init, handle, "SteamAPI_Init")
	purego.RegisterLibFunc(&c.fn.shutdown, handle, "SteamAPI_Shutdown")
	purego.RegisterLibFunc(&c.fn.runCallbacks, handle, "SteamAPI_RunCallbacks")
	purego.RegisterLibFunc(&c.fn.getUtils, handle, utilsAccessor)
	purego.RegisterLibFunc(&c.fn.getUGC, handle, ugcAccessor)
	purego.RegisterLibFunc(&c.fn.getFriends, handle, friendsAccessor)
	purego.RegisterLibFunc(&c.fn.getAppID, handle, "SteamAPI_ISteamUtils_GetAppID")
	purego.RegisterLibFunc(&c.fn.isAPICallCompleted, handle, "SteamAPI_ISteamUtils_IsAPICallCompleted")
	purego.RegisterLibFunc(&c.fn.getAPICallResult, handle, "SteamAPI_ISteamUtils_GetAPICallResult")
	purego.RegisterLibFunc(&c.fn.createItem, handle, "SteamAPI_ISteamUGC_CreateItem")
	purego.RegisterLibFunc(&c.fn.startItemUpdate, handle, "SteamAPI_ISteamUGC_StartItemUpdate")
	purego.RegisterLibFunc(&c.fn.setItemTitle, handle, "SteamAPI_ISteamUGC_SetItemTitle")
	purego.RegisterLibFunc(&c.fn.setItemDescription, handle, "SteamAPI_ISteamUGC_SetItemDescription")
	purego.RegisterLibFunc(&c.fn.setItemVisibility, handle, "SteamAPI_ISteamUGC_SetItemVisibility")
	purego.RegisterLibFunc(&c.fn.setItemContent, handle, "SteamAPI_ISteamUGC_SetItemContent")
	purego.RegisterLibFunc(&c.fn.setItemPreview, handle, "SteamAPI_ISteamUGC_SetItemPreview")
	purego.RegisterLibFunc(&c.fn.submitItemUpdate, handle, "SteamAPI_ISteamUGC_SubmitItemUpdate")
	purego.RegisterLibFunc(&c.fn.getItemUpdateProgress, handle, "SteamAPI_ISteamUGC_GetItemUpdateProgress")
	purego.RegisterLibFunc(&c.fn.getPersonaName, handle, "SteamAPI_ISteamFriends_GetPersonaName")

	return c, nil
}

// Init calls SteamAPI_Init and, only on success, resolves the ISteamUtils/
// ISteamUGC interface pointers every other method needs - confirmed live
// (against a real, installed Stellaris) that these accessors must be called
// after Init, not before, to return usable pointers. ok is false when Steam
// itself refused (not running, not logged in, no steam_appid.txt in the
// working directory) - never an error on its own, since that is an entirely
// ordinary, expected outcome worth reporting plainly rather than wrapping.
func (c *Client) Init() (ok bool) {
	if !c.fn.init() {
		return false
	}
	c.utils = c.fn.getUtils()
	c.ugc = c.fn.getUGC()
	c.friends = c.fn.getFriends()
	return true
}

// Shutdown calls SteamAPI_Shutdown. Safe to call even if Init returned false.
func (c *Client) Shutdown() {
	c.fn.shutdown()
}

// AppID returns the AppID Steam believes this process is running as - the
// direct confirmation that a working-directory steam_appid.txt actually took
// effect, expected to equal whichever AppID it named.
func (c *Client) AppID() uint32 {
	return c.fn.getAppID(c.utils)
}

// PersonaName returns the display name of whichever Steam account is
// currently signed into the local client - purely informational, never used
// to decide anything about the publish itself.
func (c *Client) PersonaName() string {
	return c.fn.getPersonaName(c.friends)
}

// CreateItemAndWait creates a new, empty Workshop item for appID and blocks
// (pumping RunCallbacks) until Steam resolves it or timeout elapses.
func (c *Client) CreateItemAndWait(appID uint32, fileType FileType, timeout time.Duration) (CreateItemResult, error) {
	call := c.fn.createItem(c.ugc, appID, int32(fileType))
	buf := make([]byte, 24) // EResult(4) + pad(4) + PublishedFileId_t(8) + bool(1) + pad(7)

	completed, err := c.awaitAPICall(call, timeout, createItemResultCallback, buf)
	if err != nil {
		return CreateItemResult{}, err
	}
	if !completed {
		return CreateItemResult{}, fmt.Errorf("steamworks: CreateItem timed out after %s", timeout)
	}
	return CreateItemResult{
		Result:                      Result(int32(binary.LittleEndian.Uint32(buf[0:4]))),
		PublishedFileID:             binary.LittleEndian.Uint64(buf[8:16]),
		NeedsToAcceptLegalAgreement: buf[16] != 0,
	}, nil
}

// StartItemUpdate begins an update session against an existing item (a
// freshly created one, or one already published) - every SetItem* call below
// only takes effect once SubmitItemUpdateAndWait commits this same handle.
func (c *Client) StartItemUpdate(appID uint32, itemID uint64) uint64 {
	return c.fn.startItemUpdate(c.ugc, appID, itemID)
}

func (c *Client) SetItemTitle(handle uint64, title string) bool {
	return c.fn.setItemTitle(c.ugc, handle, title)
}

func (c *Client) SetItemDescription(handle uint64, description string) bool {
	return c.fn.setItemDescription(c.ugc, handle, description)
}

func (c *Client) SetItemVisibility(handle uint64, visibility Visibility) bool {
	return c.fn.setItemVisibility(c.ugc, handle, int32(visibility))
}

// SetItemContent points handle's update at a local folder to upload as the
// item's content - contentFolder must exist and be readable; Steam reads it
// at SubmitItemUpdateAndWait time, not immediately.
func (c *Client) SetItemContent(handle uint64, contentFolder string) bool {
	return c.fn.setItemContent(c.ugc, handle, contentFolder)
}

// SetItemPreview points handle's update at a local image file (under 1MB,
// per Valve's own documented limit) to use as the item's preview thumbnail.
func (c *Client) SetItemPreview(handle uint64, previewFile string) bool {
	return c.fn.setItemPreview(c.ugc, handle, previewFile)
}

// SubmitItemUpdateAndWait commits every SetItem* call made against handle,
// blocking (pumping RunCallbacks) until Steam resolves it or timeout
// elapses. onProgress, if not nil, is called once per poll tick with Steam's
// own live status - never guaranteed to reach every UpdateStatus value (a
// small, content-less update can skip straight from PreparingConfig to
// CommittingChanges), so a caller should treat it as informational only.
func (c *Client) SubmitItemUpdateAndWait(handle uint64, changeNote string, timeout time.Duration, onProgress func(status UpdateStatus, processed, total uint64)) (SubmitItemUpdateResult, error) {
	call := c.fn.submitItemUpdate(c.ugc, handle, changeNote)
	buf := make([]byte, 16) // EResult(4) + bool(1) + pad(3) + PublishedFileId_t(8)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.fn.runCallbacks()

		if onProgress != nil {
			var processed, total uint64
			status := c.fn.getItemUpdateProgress(c.ugc, handle, &processed, &total)
			onProgress(UpdateStatus(status), processed, total)
		}

		var apiFailed bool
		if c.fn.isAPICallCompleted(c.utils, call, &apiFailed) {
			var gotFailed bool
			if !c.fn.getAPICallResult(c.utils, call, unsafe.Pointer(&buf[0]), int32(len(buf)), submitItemUpdateResultCallback, &gotFailed) {
				return SubmitItemUpdateResult{}, fmt.Errorf("steamworks: GetAPICallResult reported failure for SubmitItemUpdate")
			}
			return SubmitItemUpdateResult{
				Result:                      Result(int32(binary.LittleEndian.Uint32(buf[0:4]))),
				NeedsToAcceptLegalAgreement: buf[4] != 0,
				PublishedFileID:             binary.LittleEndian.Uint64(buf[8:16]),
			}, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return SubmitItemUpdateResult{}, fmt.Errorf("steamworks: SubmitItemUpdate timed out after %s", timeout)
}

// awaitAPICall is CreateItemAndWait's own poll loop, pulled out on its own
// since SubmitItemUpdateAndWait needs a second copy that also reports
// progress each tick (see above) - not worth forcing both through one
// signature just to avoid the duplication.
func (c *Client) awaitAPICall(call uint64, timeout time.Duration, expectedCallback int32, buf []byte) (completed bool, err error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.fn.runCallbacks()

		var apiFailed bool
		if c.fn.isAPICallCompleted(c.utils, call, &apiFailed) {
			var gotFailed bool
			if !c.fn.getAPICallResult(c.utils, call, unsafe.Pointer(&buf[0]), int32(len(buf)), expectedCallback, &gotFailed) {
				return true, fmt.Errorf("steamworks: GetAPICallResult reported failure")
			}
			return true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false, nil
}
