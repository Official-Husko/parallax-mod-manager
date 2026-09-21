package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

// writeWorkshopMod creates a classic-format Workshop mod (an
// "ugc_<id>.mod" descriptor) with the given remote file id.
func writeWorkshopMod(t *testing.T, modDir, remoteFileID string) {
	t.Helper()
	id := "ugc_" + remoteFileID
	writeFile(t, modDir, id+".mod", `name = "Workshop Mod `+remoteFileID+`"
path = "`+id+`"
remote_file_id = "`+remoteFileID+`"
`)
	writeFile(t, modDir, filepath.Join(id, "common", "x.txt"), "thing = {}")
}

func TestWorkshopDetailsCacheFetchesOnceThenServesFromMemory(t *testing.T) {
	modDir := t.TempDir()
	writeWorkshopMod(t, modDir, "111")
	writeWorkshopMod(t, modDir, "222")

	fetchCount := 0
	var lastRequestedIDs []string
	c := &WorkshopDetailsCache{
		fetch: func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
			fetchCount++
			lastRequestedIDs = ids
			result := make(map[string]steamapi.PublishedFileDetails, len(ids))
			for _, id := range ids {
				result[id] = steamapi.PublishedFileDetails{ID: id, Title: "Title " + id, Result: 1}
			}
			return result, nil
		},
	}

	first, err := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 1 {
		t.Fatalf("fetchCount = %d, want 1 after the first call", fetchCount)
	}
	if len(lastRequestedIDs) != 2 {
		t.Fatalf("requested ids = %v, want both mods batched into one call", lastRequestedIDs)
	}
	if len(first) != 2 || first["111"].Title != "Title 111" || first["222"].Title != "Title 222" {
		t.Errorf("first = %+v", first)
	}

	second, err := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("fetchCount = %d after a second call for the exact same mods, want still 1 (served from memory)", fetchCount)
	}
	if len(second) != 2 {
		t.Errorf("second = %+v", second)
	}
}

func TestWorkshopDetailsCacheOnlyFetchesGenuinelyMissingIDs(t *testing.T) {
	modDir := t.TempDir()
	writeWorkshopMod(t, modDir, "111")

	fetchCount := 0
	c := &WorkshopDetailsCache{
		fetch: func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
			fetchCount++
			return map[string]steamapi.PublishedFileDetails{"111": {ID: "111", Title: "First"}}, nil
		},
	}
	if _, err := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir}); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// A second, different mod is added to the same modDir - only the new
	// one should ever be requested, not a re-fetch of "111".
	writeWorkshopMod(t, modDir, "222")
	c.fetch = func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
		fetchCount++
		if len(ids) != 1 || ids[0] != "222" {
			t.Errorf("second fetch requested %v, want exactly [222]", ids)
		}
		return map[string]steamapi.PublishedFileDetails{"222": {ID: "222", Title: "Second"}}, nil
	}
	result, err := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 2 {
		t.Errorf("fetchCount = %d, want 2 (one per genuinely new id)", fetchCount)
	}
	if len(result) != 2 {
		t.Errorf("result = %+v, want both mods present", result)
	}
}

func TestWorkshopDetailsCacheIgnoresNonWorkshopMods(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "local_mod", "Local Mod", `thing = {}`)

	requested := false
	c := &WorkshopDetailsCache{
		fetch: func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
			requested = true
			return nil, nil
		},
	}
	result, err := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if requested {
		t.Error("expected no fetch for a modlist with no Workshop mods")
	}
	if len(result) != 0 {
		t.Errorf("result = %+v, want empty", result)
	}
}

func TestWorkshopDetailsCacheGetFreshAsksSteamAgain(t *testing.T) {
	modDir := t.TempDir()
	writeWorkshopMod(t, modDir, "111")

	updated := int64(100)
	fetchCount := 0
	c := &WorkshopDetailsCache{
		fetch: func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
			fetchCount++
			return map[string]steamapi.PublishedFileDetails{"111": {ID: "111", Result: 1, TimeUpdated: updated}}, nil
		},
	}

	if _, err := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir}); err != nil {
		t.Fatalf("Get: %v", err)
	}
	updated = 200
	cached, _ := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if fetchCount != 1 || cached["111"].TimeUpdated != 100 {
		t.Fatalf("Get should serve from memory: fetches %d, updated %d", fetchCount, cached["111"].TimeUpdated)
	}

	fresh, err := c.GetFresh(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil {
		t.Fatalf("GetFresh: %v", err)
	}
	if fetchCount != 2 || fresh["111"].TimeUpdated != 200 {
		t.Errorf("GetFresh: fetches %d, updated %d, want a second fetch seeing 200", fetchCount, fresh["111"].TimeUpdated)
	}
	if after, _ := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir}); after["111"].TimeUpdated != 200 {
		t.Errorf("Get after GetFresh = %d, want the refreshed 200 remembered", after["111"].TimeUpdated)
	}
}

func TestWorkshopDetailsCacheSetFetchReplacesTheFetcher(t *testing.T) {
	modDir := t.TempDir()
	writeWorkshopMod(t, modDir, "111")

	c := &WorkshopDetailsCache{}
	c.SetFetch(func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
		return map[string]steamapi.PublishedFileDetails{"111": {ID: "111", Title: "from the service", Result: 1}}, nil
	})
	got, err := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if err != nil || got["111"].Title != "from the service" {
		t.Errorf("got = %+v, err = %v", got, err)
	}
}

func TestWorkshopDetailsCacheForgetMakesTheNextGetAskAgain(t *testing.T) {
	modDir := t.TempDir()
	writeWorkshopMod(t, modDir, "111")

	answer := "not found"
	fetchCount := 0
	c := &WorkshopDetailsCache{}
	c.SetFetch(func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
		fetchCount++
		return map[string]steamapi.PublishedFileDetails{"111": {ID: "111", Title: answer}}, nil
	})
	if got, _ := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir}); got["111"].Title != "not found" {
		t.Fatalf("first = %+v", got)
	}
	answer = "found with the key"
	if got, _ := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir}); got["111"].Title != "not found" || fetchCount != 1 {
		t.Fatalf("before Forget the cache should still answer from memory: %+v (fetches %d)", got, fetchCount)
	}
	c.Forget()
	got, _ := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
	if got["111"].Title != "found with the key" || fetchCount != 2 {
		t.Errorf("after Forget: %+v (fetches %d), want a fresh fetch", got, fetchCount)
	}
}

func TestWorkshopDetailsCacheForgetDuringAFetchDoesNotBlockOrKeepTheOldAnswer(t *testing.T) {
	modDir := t.TempDir()
	writeWorkshopMod(t, modDir, "111")

	started := make(chan struct{})
	release := make(chan struct{})
	calls := 0
	c := &WorkshopDetailsCache{}
	c.SetFetch(func(ctx context.Context, ids []string) (map[string]steamapi.PublishedFileDetails, error) {
		calls++
		if calls == 1 {
			close(started)
			<-release
			return map[string]steamapi.PublishedFileDetails{"111": {ID: "111", Title: "old way"}}, nil
		}
		return map[string]steamapi.PublishedFileDetails{"111": {ID: "111", Title: "new way"}}, nil
	})

	done := make(chan map[string]steamapi.PublishedFileDetails)
	go func() {
		got, _ := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir})
		done <- got
	}()
	<-started
	c.Forget() // must return at once although the fetch above holds the cache's lock
	close(release)
	if got := <-done; got["111"].Title != "old way" {
		t.Errorf("the call in flight should still get its own answer, got %+v", got)
	}
	if got, _ := c.Get(context.Background(), testGameConfig(), Options{ModDir: modDir}); got["111"].Title != "new way" {
		t.Errorf("the old answer was kept after Forget: %+v", got)
	}
}
