package steamapi

import "fmt"

// WorkshopOpenMode is a person's chosen way of opening a Workshop item's
// page from within this app - persisted as a plain string in
// preferences.Preferences.WorkshopOpenMode, the same convention
// internal/launch.LaunchMode already uses for its own per-game choice.
type WorkshopOpenMode string

const (
	// WorkshopOpenModeBrowser opens the item's real public page
	// (WorkshopPageURL) in whatever browser the OS defaults to. The
	// long-standing behavior, and the default - see preferences.Defaults.
	WorkshopOpenModeBrowser WorkshopOpenMode = "browser"
	// WorkshopOpenModeApp opens the same item directly in the local Steam
	// client instead (WorkshopClientURL), if one is installed.
	WorkshopOpenModeApp WorkshopOpenMode = "app"
)

// WorkshopPageURL returns the public Steam Community URL for a Workshop
// item - the same address ItemPageExists itself checks, and what
// WorkshopOpenModeBrowser opens.
func WorkshopPageURL(publishedFileID string) string {
	return fmt.Sprintf("https://steamcommunity.com/sharedfiles/filedetails/?id=%s", publishedFileID)
}

// WorkshopClientURL returns the steam:// protocol URL that opens the same
// Workshop item directly in the local Steam client instead of a browser -
// confirmed as Steam's own documented protocol handler (Steamworks'
// ISteamFriends::ActivateGameOverlayToWebPage uses this exact form; Steam
// itself registers steam://url/CommunityFilePage/ as a URI scheme handler
// on install). What WorkshopOpenModeApp opens.
func WorkshopClientURL(publishedFileID string) string {
	return fmt.Sprintf("steam://url/CommunityFilePage/%s", publishedFileID)
}
