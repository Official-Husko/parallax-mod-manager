package library

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

func TestAuthorProfileCacheFetchesOnceThenServesFromMemory(t *testing.T) {
	var mu sync.Mutex
	fetchCount := 0
	c := &AuthorProfileCache{
		fetch: func(ctx context.Context, id string) (steamapi.Profile, bool, error) {
			mu.Lock()
			fetchCount++
			mu.Unlock()
			return steamapi.Profile{Name: "Author " + id}, true, nil
		},
	}

	first, err := c.Get(context.Background(), []string{"111", "222"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 2 {
		t.Fatalf("fetchCount = %d, want 2 after the first call", fetchCount)
	}
	if len(first) != 2 || first["111"].Name != "Author 111" || first["222"].Name != "Author 222" {
		t.Errorf("first = %+v", first)
	}

	second, err := c.Get(context.Background(), []string{"111", "222"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 2 {
		t.Errorf("fetchCount = %d after a second call for the exact same ids, want still 2 (served from memory)", fetchCount)
	}
	if len(second) != 2 {
		t.Errorf("second = %+v", second)
	}
}

func TestAuthorProfileCacheOnlyFetchesGenuinelyMissingIDs(t *testing.T) {
	var mu sync.Mutex
	fetchCount := 0
	c := &AuthorProfileCache{
		fetch: func(ctx context.Context, id string) (steamapi.Profile, bool, error) {
			mu.Lock()
			fetchCount++
			mu.Unlock()
			return steamapi.Profile{Name: "Author " + id}, true, nil
		},
	}
	if _, err := c.Get(context.Background(), []string{"111"}); err != nil {
		t.Fatalf("Get: %v", err)
	}

	result, err := c.Get(context.Background(), []string{"111", "222"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 2 {
		t.Errorf("fetchCount = %d, want 2 (one per genuinely new id, none for the already-cached one)", fetchCount)
	}
	if len(result) != 2 {
		t.Errorf("result = %+v, want both ids present", result)
	}
}

func TestAuthorProfileCacheDeduplicatesWithinOneCall(t *testing.T) {
	var mu sync.Mutex
	requested := map[string]int{}
	c := &AuthorProfileCache{
		fetch: func(ctx context.Context, id string) (steamapi.Profile, bool, error) {
			mu.Lock()
			requested[id]++
			mu.Unlock()
			return steamapi.Profile{Name: "Author " + id}, true, nil
		},
	}

	// The same creator id can legitimately appear more than once (several
	// mods sharing one author) - the caller is expected to already
	// deduplicate before calling Get, but Get must not fetch an id twice
	// even if it didn't.
	if _, err := c.Get(context.Background(), []string{"111", "111", "111"}); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if requested["111"] != 1 {
		t.Errorf("requested[111] = %d, want exactly 1", requested["111"])
	}
}

func TestAuthorProfileCacheFailedLookupIsAbsentNotError(t *testing.T) {
	c := &AuthorProfileCache{
		fetch: func(ctx context.Context, id string) (steamapi.Profile, bool, error) {
			if id == "bad" {
				return steamapi.Profile{}, false, nil
			}
			return steamapi.Profile{Name: "Author " + id}, true, nil
		},
	}

	result, err := c.Get(context.Background(), []string{"good", "bad"})
	if err != nil {
		t.Fatalf("Get: %v, want no error for a normal failed lookup", err)
	}
	if len(result) != 1 {
		t.Fatalf("result = %+v, want only the good id present", result)
	}
	if _, ok := result["bad"]; ok {
		t.Error("result contains \"bad\", want it absent")
	}
}

func TestAuthorProfileCacheFetchErrorIsAbsentNotFatal(t *testing.T) {
	c := &AuthorProfileCache{
		fetch: func(ctx context.Context, id string) (steamapi.Profile, bool, error) {
			return steamapi.Profile{}, false, fmt.Errorf("transient steam failure")
		},
	}

	result, err := c.Get(context.Background(), []string{"111"})
	if err != nil {
		t.Fatalf("Get: %v, want a per-item fetch error to not fail the whole call", err)
	}
	if len(result) != 0 {
		t.Errorf("result = %+v, want empty", result)
	}
}

func TestAuthorProfileCacheIgnoresEmptyIDs(t *testing.T) {
	requested := false
	c := &AuthorProfileCache{
		fetch: func(ctx context.Context, id string) (steamapi.Profile, bool, error) {
			requested = true
			return steamapi.Profile{}, false, nil
		},
	}

	result, err := c.Get(context.Background(), []string{"", ""})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if requested {
		t.Error("expected no fetch for empty ids")
	}
	if len(result) != 0 {
		t.Errorf("result = %+v, want empty", result)
	}
}
