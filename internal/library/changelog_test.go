package library

import (
	"context"
	"fmt"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

func TestChangelogCacheFetchesOnceThenServesFromMemory(t *testing.T) {
	fetchCount := 0
	c := &ChangelogCache{
		fetch: func(ctx context.Context, id string) ([]steamapi.ChangelogEntry, error) {
			fetchCount++
			return []steamapi.ChangelogEntry{{Headline: "Update: 1 Jan", Body: "First"}}, nil
		},
	}

	first, err := c.Get(context.Background(), "111")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 1 {
		t.Fatalf("fetchCount = %d, want 1", fetchCount)
	}
	if len(first) != 1 || first[0].Body != "First" {
		t.Errorf("first = %+v", first)
	}

	second, err := c.Get(context.Background(), "111")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("fetchCount = %d after a second call for the same id, want still 1 (served from memory)", fetchCount)
	}
	if len(second) != 1 {
		t.Errorf("second = %+v", second)
	}
}

func TestChangelogCacheFetchesEachDistinctIDOnce(t *testing.T) {
	requested := map[string]int{}
	c := &ChangelogCache{
		fetch: func(ctx context.Context, id string) ([]steamapi.ChangelogEntry, error) {
			requested[id]++
			return []steamapi.ChangelogEntry{{Headline: "Update for " + id}}, nil
		},
	}

	if _, err := c.Get(context.Background(), "111"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := c.Get(context.Background(), "222"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := c.Get(context.Background(), "111"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if requested["111"] != 1 || requested["222"] != 1 {
		t.Errorf("requested = %+v, want exactly one fetch per distinct id", requested)
	}
}

func TestChangelogCacheFetchErrorIsNotCached(t *testing.T) {
	attempt := 0
	c := &ChangelogCache{
		fetch: func(ctx context.Context, id string) ([]steamapi.ChangelogEntry, error) {
			attempt++
			if attempt == 1 {
				return nil, fmt.Errorf("transient steam failure")
			}
			return []steamapi.ChangelogEntry{{Headline: "Recovered"}}, nil
		},
	}

	if _, err := c.Get(context.Background(), "111"); err == nil {
		t.Fatal("expected the first call's error to surface")
	}
	result, err := c.Get(context.Background(), "111")
	if err != nil {
		t.Fatalf("Get: %v, want a retry after a prior failure to succeed", err)
	}
	if len(result) != 1 || result[0].Headline != "Recovered" {
		t.Errorf("result = %+v", result)
	}
	if attempt != 2 {
		t.Errorf("attempt = %d, want 2 (a failed fetch must not be cached)", attempt)
	}
}
