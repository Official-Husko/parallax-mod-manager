package dlcstore

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/dlc"
	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Dir: t.TempDir(), GameKey: "stellaris"}
	cf := CacheFile{
		FetchedAt: 1700000000,
		ByAppID: map[string]StoreData{
			"716670": {SteamAppID: "716670", Name: "Apocalypse", ReleaseDate: "Feb 26, 2021"},
		},
	}
	if err := s.Save(cf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.FetchedAt != cf.FetchedAt {
		t.Errorf("FetchedAt = %d, want %d", got.FetchedAt, cf.FetchedAt)
	}
	if got.ByAppID["716670"].Name != "Apocalypse" {
		t.Errorf("ByAppID = %+v", got.ByAppID)
	}
}

func TestLoadMissingFileReturnsRealEmptyCacheNeedingRefresh(t *testing.T) {
	s := Store{Dir: t.TempDir(), GameKey: "stellaris"}
	cf, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cf.ByAppID == nil {
		t.Error("ByAppID is nil, want a real empty map")
	}
	if !s.NeedsRefresh(cf) {
		t.Error("NeedsRefresh = false for a never-fetched cache, want true")
	}
}

func TestNeedsRefreshTrueWhenOlderThanMaxAge(t *testing.T) {
	s := Store{Dir: t.TempDir(), GameKey: "stellaris"}
	stale := CacheFile{FetchedAt: time.Now().Add(-25 * time.Hour).Unix(), ByAppID: map[string]StoreData{}}
	if !s.NeedsRefresh(stale) {
		t.Error("NeedsRefresh = false for a 25-hour-old cache, want true")
	}
}

func TestNeedsRefreshFalseWhenFresh(t *testing.T) {
	s := Store{Dir: t.TempDir(), GameKey: "stellaris"}
	fresh := CacheFile{FetchedAt: time.Now().Add(-1 * time.Hour).Unix(), ByAppID: map[string]StoreData{}}
	if s.NeedsRefresh(fresh) {
		t.Error("NeedsRefresh = true for a 1-hour-old cache, want false")
	}
}

// withFakeAppDetails swaps fetchAppDetails for a fake, restoring the real
// one after the test - no real network call is ever made in this
// package's tests.
func withFakeAppDetails(t *testing.T, fake func(ctx context.Context, appID string) (steamapi.AppDetails, bool, error)) {
	t.Helper()
	restore := fetchAppDetails
	fetchAppDetails = fake
	t.Cleanup(func() { fetchAppDetails = restore })
}

func TestRefreshSkipsEntriesWithNoSteamID(t *testing.T) {
	var mu sync.Mutex
	requested := []string{}
	withFakeAppDetails(t, func(ctx context.Context, appID string) (steamapi.AppDetails, bool, error) {
		mu.Lock()
		requested = append(requested, appID)
		mu.Unlock()
		return steamapi.AppDetails{}, false, nil
	})

	entries := []dlc.Entry{
		{ID: "no_steam_id", Name: "Local Only"},
	}
	cf := Refresh(context.Background(), "", entries)
	if len(requested) != 0 {
		t.Errorf("requested = %v, want none - no entry had a SteamID and no base game id was given", requested)
	}
	if len(cf.ByAppID) != 0 {
		t.Errorf("ByAppID = %+v, want empty", cf.ByAppID)
	}
	if cf.FetchedAt == 0 {
		t.Error("FetchedAt = 0, want the real current time even when nothing was fetched")
	}
}

func TestRefreshFetchesLocalEntriesBySteamID(t *testing.T) {
	withFakeAppDetails(t, func(ctx context.Context, appID string) (steamapi.AppDetails, bool, error) {
		if appID != "716670" {
			t.Errorf("requested appID = %q, want 716670", appID)
		}
		return steamapi.AppDetails{Name: "Apocalypse"}, true, nil
	})

	entries := []dlc.Entry{{ID: "dlc017_apocalypse", SteamID: "716670", Installed: true}}
	cf := Refresh(context.Background(), "", entries)
	if cf.ByAppID["716670"].Name != "Apocalypse" {
		t.Errorf("ByAppID = %+v", cf.ByAppID)
	}
}

// TestRefreshCarriesComingSoonAndScreenshots pins the real feature this
// exists for: an unreleased DLC (see internal/steamapi's real "coming
// soon" fixture) has no other real detail this project could ever show -
// nothing local to read, since it doesn't exist to install - so both its
// real ComingSoon flag and real screenshot URLs must survive into the
// persisted cache.
func TestRefreshCarriesComingSoonAndScreenshots(t *testing.T) {
	withFakeAppDetails(t, func(ctx context.Context, appID string) (steamapi.AppDetails, bool, error) {
		return steamapi.AppDetails{
			Name:        "Scenario Pack 1",
			ComingSoon:  true,
			ReleaseDate: "Coming soon",
			Screenshots: []string{"https://example.com/ss1.jpg", "https://example.com/ss2.jpg"},
		}, true, nil
	})

	entries := []dlc.Entry{{ID: "dlc999", SteamID: "4241450", Installed: true}}
	cf := Refresh(context.Background(), "", entries)
	got := cf.ByAppID["4241450"]
	if !got.ComingSoon {
		t.Error("ComingSoon = false, want true")
	}
	if len(got.Screenshots) != 2 {
		t.Errorf("Screenshots = %v, want 2", got.Screenshots)
	}
}

// TestRefreshDiscoversCatalogDLCNotInstalledLocally pins the real feature
// this exists for: a DLC the base game's own Steam catalog lists but that
// isn't installed here still gets real Store data fetched for it, so
// library.MergeDLCCatalog can show it (never toggleable, just informational).
func TestRefreshDiscoversCatalogDLCNotInstalledLocally(t *testing.T) {
	var mu sync.Mutex
	requested := map[string]bool{}
	withFakeAppDetails(t, func(ctx context.Context, appID string) (steamapi.AppDetails, bool, error) {
		mu.Lock()
		requested[appID] = true
		mu.Unlock()
		if appID == "281990" {
			return steamapi.AppDetails{Name: "Stellaris", DLCAppIDs: []string{"716670", "554350"}}, true, nil
		}
		return steamapi.AppDetails{Name: "DLC " + appID}, true, nil
	})

	// Only 554350 is actually installed locally; 716670 is catalog-only.
	entries := []dlc.Entry{{ID: "dlc013_horizon_signal", SteamID: "554350", Installed: true}}
	cf := Refresh(context.Background(), "281990", entries)

	if !requested["281990"] {
		t.Error("expected the base game's own AppDetails to be fetched to discover its catalog")
	}
	if !requested["716670"] {
		t.Error("expected the catalog-only DLC (716670) to also get a real fetch")
	}
	if len(cf.ByAppID) != 2 {
		t.Fatalf("ByAppID = %+v, want both the installed and the catalog-only DLC", cf.ByAppID)
	}
	if cf.ByAppID["716670"].Name != "DLC 716670" {
		t.Errorf("ByAppID[716670] = %+v", cf.ByAppID["716670"])
	}
}

func TestRefreshDedeuplicatesCatalogIDAlreadyCoveredLocally(t *testing.T) {
	var mu sync.Mutex
	fetchCount := 0
	withFakeAppDetails(t, func(ctx context.Context, appID string) (steamapi.AppDetails, bool, error) {
		mu.Lock()
		fetchCount++
		mu.Unlock()
		if appID == "281990" {
			return steamapi.AppDetails{DLCAppIDs: []string{"716670"}}, true, nil
		}
		return steamapi.AppDetails{Name: "Apocalypse"}, true, nil
	})

	entries := []dlc.Entry{{ID: "dlc017_apocalypse", SteamID: "716670", Installed: true}}
	cf := Refresh(context.Background(), "281990", entries)

	// Base game (1) + the one shared DLC id (1, not fetched twice) = 2.
	if fetchCount != 2 {
		t.Errorf("fetchCount = %d, want 2 (base game once, the shared DLC id once, not twice)", fetchCount)
	}
	if len(cf.ByAppID) != 1 {
		t.Errorf("ByAppID = %+v, want exactly 1 entry", cf.ByAppID)
	}
}

func TestSaveWritesOnePersistentFilePerGame(t *testing.T) {
	dir := t.TempDir()
	stellaris := Store{Dir: dir, GameKey: "stellaris"}
	eu4 := Store{Dir: dir, GameKey: "eu4"}
	if err := stellaris.Save(CacheFile{FetchedAt: 1, ByAppID: map[string]StoreData{}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := eu4.Save(CacheFile{FetchedAt: 2, ByAppID: map[string]StoreData{}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "stellaris_dlc_store.json")); err != nil {
		t.Errorf("expected a per-game file for stellaris: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "eu4_dlc_store.json")); err != nil {
		t.Errorf("expected a per-game file for eu4: %v", err)
	}

	got, err := stellaris.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.FetchedAt != 1 {
		t.Errorf("stellaris FetchedAt = %d, want 1 (must not read eu4's file)", got.FetchedAt)
	}
}
