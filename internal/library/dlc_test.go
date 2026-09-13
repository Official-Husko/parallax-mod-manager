package library

import (
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/dlc"
	"github.com/Official-Husko/parallax-mod-manager/internal/dlcstore"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func TestListDLCRejectsNonClassicGames(t *testing.T) {
	cfg := testGameConfig()
	cfg.DescriptorType = mod.DescriptorJSONv1

	if _, err := ListDLC(cfg); err == nil {
		t.Fatal("expected an error for a JSON-descriptor game")
	}
}

func TestListDLCReturnsEmptyForUndetectedInstall(t *testing.T) {
	// testGameConfig's install-detection signature files won't exist
	// anywhere real on the test machine, so DetectInstall reports false -
	// this must return an empty list, not an error.
	entries, err := ListDLC(testGameConfig())
	if err != nil {
		t.Fatalf("ListDLC: %v", err)
	}
	if entries == nil {
		t.Error("entries is nil, want a real empty slice (marshals as JSON null, not [])")
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %+v", entries)
	}
}

func TestMergeDLCCatalogAddsCatalogOnlyEntriesAsNotInstalled(t *testing.T) {
	local := []dlc.Entry{
		{ID: "dlc017_apocalypse", SteamID: "716670", Name: "Apocalypse", Installed: true},
	}
	cache := dlcstore.CacheFile{
		ByAppID: map[string]dlcstore.StoreData{
			"716670":  {SteamAppID: "716670", Name: "Apocalypse"},   // already covered locally
			"1522090": {SteamAppID: "1522090", Name: "Federations"}, // catalog-only
		},
	}

	merged := MergeDLCCatalog(local, cache)
	if len(merged) != 2 {
		t.Fatalf("merged = %+v, want 2 entries", merged)
	}

	var catalogOnly *dlc.Entry
	for i := range merged {
		if merged[i].SteamID == "1522090" {
			catalogOnly = &merged[i]
		}
	}
	if catalogOnly == nil {
		t.Fatal("expected a synthetic entry for the catalog-only DLC")
	}
	if catalogOnly.Installed {
		t.Error("Installed = true, want false for a catalog-only entry")
	}
	if catalogOnly.ID != "" {
		t.Errorf("ID = %q, want empty - there's no local folder for a catalog-only entry", catalogOnly.ID)
	}
	if catalogOnly.Name != "Federations" {
		t.Errorf("Name = %q, want the real Store name", catalogOnly.Name)
	}
}

func TestMergeDLCCatalogNeverDuplicatesLocalEntry(t *testing.T) {
	local := []dlc.Entry{
		{ID: "dlc017_apocalypse", SteamID: "716670", Name: "Apocalypse", Installed: true},
	}
	cache := dlcstore.CacheFile{
		ByAppID: map[string]dlcstore.StoreData{
			"716670": {SteamAppID: "716670", Name: "Apocalypse"},
		},
	}

	merged := MergeDLCCatalog(local, cache)
	if len(merged) != 1 {
		t.Fatalf("merged = %+v, want exactly 1 entry (no duplicate for the already-local one)", merged)
	}
	if !merged[0].Installed {
		t.Error("the real local entry's Installed must stay true")
	}
}

func TestMergeDLCCatalogWithEmptyCacheReturnsLocalUnchanged(t *testing.T) {
	local := []dlc.Entry{
		{ID: "dlc017_apocalypse", SteamID: "716670", Name: "Apocalypse", Installed: true},
	}
	merged := MergeDLCCatalog(local, dlcstore.CacheFile{})
	if len(merged) != 1 || merged[0].ID != "dlc017_apocalypse" {
		t.Errorf("merged = %+v, want local unchanged", merged)
	}
}
