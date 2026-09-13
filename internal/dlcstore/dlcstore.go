// Package dlcstore persists real Steam Store data (header image, release
// date, short description) for a game's DLC - both installed and merely
// known to the base game's own official Steam catalog - refreshed at most
// once a day - see MaxAge - since none of it changes minute to minute and
// it's the one part of this project that calls out to a third party at
// all. Keyed by Steam Store app id (StoreData.SteamAppID), the one
// identifier both an installed and a catalog-only DLC always have.
// Deliberately doesn't carry price - see steamapi.AppDetails' doc comment.
package dlcstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/dlc"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// MaxAge is how long a cached fetch stays valid before Store.NeedsRefresh
// reports true - once a day, since DLC listings (name, price, header
// image) don't meaningfully change more often than that.
const MaxAge = 24 * time.Hour

// maxConcurrentFetches bounds how many Store requests Refresh makes at
// once. Steam is a shared third-party service, not local disk - unlike
// this project's other parallel work (e.g. library.ModSizes), this stays
// deliberately low rather than scaling to runtime.NumCPU().
const maxConcurrentFetches = 5

// fetchAppDetails is steamapi.GetAppDetails - a var, not a direct call, so
// this package's own tests can substitute a fake that never makes a real
// network request (the same pattern steamapi's own tests use to swap in a
// local httptest.Server via its package-private URL vars).
var fetchAppDetails = steamapi.GetAppDetails

// StoreData is one DLC's real Steam Store listing.
type StoreData struct {
	// SteamAppID is this listing's own Steam Store app id - carried on the
	// struct itself, not just as ByAppID's map key, so a slice of these
	// round-trips through the Wails/JS boundary self-identifying: a bound
	// method returning map[string]StoreData never generates StoreData's
	// own TS class at all (confirmed against this project's installed
	// Wails version - a struct only reachable through a map value type
	// isn't walked for class generation, unlike one reachable through a
	// slice), so every Wails-bound method in this project returns a
	// slice, never a map, of any struct type.
	SteamAppID       string
	Name             string
	ShortDescription string
	HeaderImage      string
	ReleaseDate      string
	// ComingSoon is Steam's own real flag for "not released yet" - the
	// reliable way to know that, rather than trying to detect it from
	// ReleaseDate's text (which is just "Coming soon" in that case, but
	// isn't guaranteed stable wording). Most useful for a catalog-only
	// DLC (see library.MergeDLCCatalog) that isn't installed because it
	// simply doesn't exist to install yet.
	ComingSoon bool
	// Screenshots are real Steam preview image URLs - for a catalog-only
	// DLC especially, this is the only real content this project can
	// show at all (there's nothing local to read anything from).
	Screenshots []string
}

// CacheFile is the on-disk persisted shape - one per game.
type CacheFile struct {
	// FetchedAt is when ByAppID was last successfully refreshed, Unix
	// seconds. Zero means never fetched.
	FetchedAt int64
	// ByAppID covers every DLC Refresh could find real Store data for -
	// both ones installed locally (via their own .dlc file's steam_id)
	// and ones merely listed in the base game's own official Steam
	// catalog but not installed here. library.MergeDLCCatalog is what
	// turns "in this map but not installed" into a real, shown-but-
	// never-toggleable Entry.
	ByAppID map[string]StoreData
}

// Store persists one game's DLC Store-data cache as
// "<Dir>/<GameKey>_dlc_store.json".
type Store struct {
	Dir     string // required - the caller resolves the real cache directory, this package never does
	GameKey string // required
}

func (s Store) filename() string {
	return s.GameKey + "_dlc_store.json"
}

// Load reads the persisted cache, or a real empty CacheFile{} (FetchedAt:
// 0, so NeedsRefresh reports true) if nothing has been fetched yet - not
// an error, matching this project's "missing is fine, don't fabricate"
// handling elsewhere.
func (s Store) Load() (CacheFile, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, s.filename()))
	if os.IsNotExist(err) {
		return CacheFile{ByAppID: map[string]StoreData{}}, nil
	}
	if err != nil {
		return CacheFile{}, err
	}
	var cf CacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return CacheFile{}, err
	}
	if cf.ByAppID == nil {
		cf.ByAppID = map[string]StoreData{}
	}
	return cf, nil
}

// Save atomically writes cf to disk.
func (s Store) Save(cf CacheFile) error {
	_, err := atomicfile.WriteJSON(s.Dir, s.filename(), cf)
	return err
}

// SaveRefreshed persists a freshly computed Refresh result, unless it
// fetched zero entries while the existing on-disk cache already has real
// ones - a real, fully populated DLC catalog doesn't drop to nothing
// between one day and the next; a completely empty result almost always
// means every single fetch failed (a network hiccup, rate limiting), not
// that the DLC genuinely disappeared. Saving it anyway would silently
// replace good data with an empty cache that then looks "fresh" (a new
// FetchedAt) for a full MaxAge, blanking every real detail (header image,
// release year, description, coming-soon status) until the next scheduled
// refresh a day later. Keeping the last known-good data for one more
// cycle instead is strictly safer - the next refresh attempt will still
// happen on schedule.
//
// Returns whether a save actually happened, so a caller can skip emitting
// a "data changed" event when nothing did.
func (s Store) SaveRefreshed(cf CacheFile) (saved bool, err error) {
	if len(cf.ByAppID) == 0 {
		if existing, loadErr := s.Load(); loadErr == nil && len(existing.ByAppID) > 0 {
			return false, nil
		}
	}
	if err := s.Save(cf); err != nil {
		return false, err
	}
	return true, nil
}

// NeedsRefresh reports whether cf is missing or older than MaxAge.
func (s Store) NeedsRefresh(cf CacheFile) bool {
	if cf.FetchedAt == 0 {
		return true
	}
	return time.Since(time.Unix(cf.FetchedAt, 0)) > MaxAge
}

// Refresh fetches real Store data for every one of localEntries' SteamIDs,
// plus every DLC in baseGameSteamAppID's own official Steam catalog (its
// "dlc" list - see steamapi.AppDetails.DLCAppIDs) that isn't already
// covered - so a DLC that exists in Steam's catalog but isn't installed
// here still gets real Store data to show (never toggleable, since
// there's no local folder for it - see library.MergeDLCCatalog).
//
// Every fetch (including the base game's own, to discover its catalog) is
// bounded to maxConcurrentFetches at once - Steam is a shared third
// party, not local disk. A single DLC's fetch failing (network hiccup,
// rate-limiting, a delisted app) doesn't fail the whole refresh - it's
// just missing from the result, matching this project's non-fatal-per-
// item philosophy elsewhere (scan.Scan, ModSizes). baseGameSteamAppID may
// be empty (catalog discovery is then simply skipped, not an error).
// Returns a real CacheFile with FetchedAt set to now, ready to Save.
func Refresh(ctx context.Context, baseGameSteamAppID string, localEntries []dlc.Entry) CacheFile {
	ids := map[string]bool{}
	for _, e := range localEntries {
		if e.SteamID != "" {
			ids[e.SteamID] = true
		}
	}
	if baseGameSteamAppID != "" {
		if base, ok, err := fetchAppDetails(ctx, baseGameSteamAppID); err == nil && ok {
			for _, id := range base.DLCAppIDs {
				ids[id] = true
			}
		}
	}

	var mu sync.Mutex
	byAppID := map[string]StoreData{}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentFetches)
	for id := range ids {
		g.Go(func() error {
			details, ok, err := fetchAppDetails(gctx, id)
			if err != nil || !ok {
				return nil
			}
			mu.Lock()
			byAppID[id] = StoreData{
				SteamAppID:       id,
				Name:             details.Name,
				ShortDescription: details.ShortDescription,
				HeaderImage:      details.HeaderImage,
				ReleaseDate:      details.ReleaseDate,
				ComingSoon:       details.ComingSoon,
				Screenshots:      details.Screenshots,
			}
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // every real error is already swallowed per-item above

	return CacheFile{FetchedAt: time.Now().Unix(), ByAppID: byAppID}
}
