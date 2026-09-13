package library

import (
	"fmt"

	"github.com/Official-Husko/parallax-mod-manager/internal/dlc"
	"github.com/Official-Husko/parallax-mod-manager/internal/dlcstore"
	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// ListDLC lists cfg's real installed DLC, for the DLC screen's toggle list.
//
// Classic-descriptor games only - see docs/game-launching.md: DLC disabling
// for Paradox-Launcher/JSON-format games isn't confirmed against a real
// install yet, matching this project's other classic-only precedents
// (GeneratePatch, EnsureWorkshopStub).
//
// A game that isn't currently detected as installed returns an empty list,
// not an error - matching scan.Scan's own "nothing there yet is fine"
// handling, since DetectGame already reports Installed separately.
func ListDLC(cfg game.GameConfig) ([]dlc.Entry, error) {
	if cfg.DescriptorType != mod.DescriptorClassic {
		// No "library: " prefix - meant to be read directly by a user, not
		// a log (same reasoning as mod.Mod.ContentMissingError).
		return nil, fmt.Errorf("DLC toggling isn't available for %s yet", cfg.DisplayName)
	}
	installDir, installed := cfg.DetectInstall()
	if !installed {
		return []dlc.Entry{}, nil
	}
	return dlc.Discover(installDir)
}

// MergeDLCCatalog adds a synthetic, non-installed Entry for every DLC in
// cache that isn't already among local's own SteamIDs - the base game's
// full official Steam catalog (discovered and cached by
// dlcstore.Refresh/Store) minus what's actually found locally. A caller
// can show these too (name, and via cache's own data - header image), just
// never let them be toggled: there's no local folder for disabled_dlcs to
// actually reference.
func MergeDLCCatalog(local []dlc.Entry, cache dlcstore.CacheFile) []dlc.Entry {
	known := make(map[string]bool, len(local))
	for _, e := range local {
		if e.SteamID != "" {
			known[e.SteamID] = true
		}
	}

	merged := append([]dlc.Entry{}, local...)
	for appID, data := range cache.ByAppID {
		if known[appID] {
			continue
		}
		merged = append(merged, dlc.Entry{
			SteamID:   appID,
			Name:      data.Name,
			Installed: false,
		})
	}
	return merged
}
