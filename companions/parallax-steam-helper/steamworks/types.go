// Package steamworks is a minimal, hand-written binding to the small slice of
// Steamworks' flat C API this project needs to publish a Workshop item - written
// fresh against real, confirmed function signatures (verified with `nm` against a
// real, installed game's own libsteam_api.so, cross-checked for parameter types
// against a reference SDK header - see the parent project's docs/workshop-upload.md
// for exactly how and why), never copied from Valve's own header text.
//
// Uses github.com/ebitengine/purego (dlopen/dlsym, no cgo) rather than cgo,
// specifically so this companion keeps cross-compiling for Windows from a
// non-Windows build machine with a plain `go build` - matching every other
// companion under companions/ (see the parent project's build.sh, which builds
// every companion for both linux/amd64 and windows/amd64 with no C toolchain
// involved).
package steamworks

// Result is the small subset of Valve's own EResult this project actually
// branches on - anything else is surfaced to the caller as its raw int value.
type Result int32

const (
	ResultOK           Result = 1
	ResultFail         Result = 2
	ResultInvalidParam Result = 8
	ResultFileNotFound Result = 9
	ResultAccessDenied Result = 15
	ResultBanned       Result = 17
)

// Visibility mirrors Valve's ERemoteStoragePublishedFileVisibility.
type Visibility int32

const (
	VisibilityPublic      Visibility = 0
	VisibilityFriendsOnly Visibility = 1
	VisibilityPrivate     Visibility = 2
	VisibilityUnlisted    Visibility = 3
)

// FileType mirrors Valve's EWorkshopFileType - only the one value this project
// ever creates (a normal, subscribable Workshop item).
type FileType int32

const FileTypeCommunity FileType = 0

// UpdateStatus mirrors Valve's EItemUpdateStatus, as reported while an item
// update is in flight (see Client.SubmitItemUpdateAndWait's onProgress).
type UpdateStatus int32

const (
	UpdateStatusInvalid           UpdateStatus = 0
	UpdateStatusPreparingConfig   UpdateStatus = 1
	UpdateStatusPreparingContent  UpdateStatus = 2
	UpdateStatusUploadingContent  UpdateStatus = 3
	UpdateStatusUploadingPreview  UpdateStatus = 4
	UpdateStatusCommittingChanges UpdateStatus = 5
)

// kSteamUGCCallbacks is ISteamUGC's callback-id base (confirmed against the
// reference header's steam_api_internal.h) - CreateItemResult_t and
// SubmitItemUpdateResult_t are this base plus a small, fixed, Valve-defined
// offset, used to tell GetAPICallResult which result shape to decode.
const kSteamUGCCallbacks = 3400

const (
	createItemResultCallback       = kSteamUGCCallbacks + 3
	submitItemUpdateResultCallback = kSteamUGCCallbacks + 4
)

// CreateItemResult is Valve's CreateItemResult_t, decoded from the raw bytes
// GetAPICallResult fills in - see decodeCreateItemResult. Its field order and
// offsets (EResult, then padding, then the 8-byte PublishedFileId_t, then a
// trailing bool) come from the struct's natural x86-64 alignment, confirmed
// against the reference header's field declarations, not assumed.
type CreateItemResult struct {
	Result                      Result
	PublishedFileID             uint64
	NeedsToAcceptLegalAgreement bool
}

// SubmitItemUpdateResult is Valve's SubmitItemUpdateResult_t, decoded the same
// way - note its field order genuinely differs from CreateItemResult_t (the
// bool comes second here, before the PublishedFileId_t, not last).
type SubmitItemUpdateResult struct {
	Result                      Result
	NeedsToAcceptLegalAgreement bool
	PublishedFileID             uint64
}
