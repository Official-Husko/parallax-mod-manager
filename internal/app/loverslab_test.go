package app

import (
	"context"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
)

func TestFindCategoryByName(t *testing.T) {
	categories := []loverslab.Category{
		{Name: "Skyrim: Special Edition", Depth: 0},
		{Name: "Other", Depth: 0},
		{Name: "Paradox Games", Depth: 1},
	}
	got := findCategoryByName(categories, "paradox games")
	if got == nil || got.Name != "Paradox Games" {
		t.Fatalf("got %v, want the Paradox Games category", got)
	}
	// Case and surrounding whitespace in the real site's own markup should not matter.
	got2 := findCategoryByName([]loverslab.Category{{Name: "  PARADOX GAMES  "}}, "paradox games")
	if got2 == nil {
		t.Error("expected a case/whitespace-insensitive match")
	}
	if findCategoryByName(categories, "no such category") != nil {
		t.Error("expected no match for a name that is not present")
	}
}

func TestBuildBrowseSidebarPutsAllFirstThenRealSubcategories(t *testing.T) {
	paradox := loverslab.Category{ID: 194, Name: "Paradox Games", URL: "https://www.loverslab.com/files/category/194-paradox-games/", Files: 675, Depth: 1}
	subs := []loverslab.Category{
		{Name: "Crusader Kings 2", URL: "https://www.loverslab.com/files/category/162-crusader-kings-2/", Files: 149},
		{Name: "Crusader Kings 3", URL: "https://www.loverslab.com/files/category/263-crusader-kings-3/", Files: 277},
		{Name: "Stellaris", URL: "https://www.loverslab.com/files/category/192-stellaris/", Files: 249},
	}

	got := buildBrowseSidebar(paradox, subs)

	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4 (All + 3 subcategories): %+v", len(got), got)
	}
	if got[0].Name != "All" || got[0].URL != paradox.URL || got[0].Files != 675 || got[0].Depth != 0 {
		t.Errorf("first entry = %+v, want the relabeled Paradox Games category as All at depth 0", got[0])
	}
	for i, want := range subs {
		row := got[i+1]
		if row.Name != want.Name || row.URL != want.URL || row.Files != want.Files {
			t.Errorf("entry %d = %+v, want %+v", i+1, row, want)
		}
		if row.Depth != 1 {
			t.Errorf("entry %d Depth = %d, want 1", i+1, row.Depth)
		}
	}
}

func TestBuildBrowseSidebarWithNoSubcategoriesIsJustAll(t *testing.T) {
	paradox := loverslab.Category{Name: "Paradox Games", URL: "https://www.loverslab.com/files/category/194-paradox-games/", Files: 675}
	got := buildBrowseSidebar(paradox, nil)
	if len(got) != 1 || got[0].Name != "All" {
		t.Fatalf("got %+v, want just All", got)
	}
}

func TestBuildBrowseSidebarDropsTheSelfReferencingRow(t *testing.T) {
	// Viewing a category's own page can re-list that category itself in its own sidebar -
	// it must not show up a second time as if it were one of its own subcategories.
	paradox := loverslab.Category{Name: "Paradox Games", URL: "https://www.loverslab.com/files/category/194-paradox-games/", Files: 675}
	subs := []loverslab.Category{
		{Name: "Paradox Games", URL: paradox.URL, Files: 675},
		{Name: "Stellaris", URL: "https://www.loverslab.com/files/category/192-stellaris/", Files: 249},
	}
	got := buildBrowseSidebar(paradox, subs)
	if len(got) != 2 {
		t.Fatalf("got %+v, want All + Stellaris only", got)
	}
	if got[1].Name != "Stellaris" {
		t.Errorf("got[1] = %+v, want Stellaris", got[1])
	}
}

// --- ensureLoversLabSession: reuse, caching, and the fallback chain ---
// All scripted (newLoversLabApp), never a real request - see loverslab_settings_test.go.

func TestEnsureLoversLabSessionReusesARecentlyVerifiedClientWithoutReVerifying(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	firstClient := a.loverslab.client

	verifyCalls := 0
	a.loverslab.verify = func(ctx context.Context, c *loverslab.Client) (bool, error) {
		verifyCalls++
		return true, nil
	}

	client, err := a.ensureLoversLabSession(context.Background())
	if err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if client != firstClient {
		t.Error("expected the same in-memory client to be reused")
	}
	if verifyCalls != 0 {
		t.Errorf("verify called %d times, want 0 (still within sessionRecheckInterval)", verifyCalls)
	}
}

func TestEnsureLoversLabSessionReVerifiesOnceTheRecheckIntervalHasPassed(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	a.loverslab.verifiedAt = time.Now().Add(-sessionRecheckInterval - time.Minute)

	verifyCalls := 0
	a.loverslab.verify = func(ctx context.Context, c *loverslab.Client) (bool, error) {
		verifyCalls++
		return true, nil
	}

	if _, err := a.ensureLoversLabSession(context.Background()); err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if verifyCalls != 1 {
		t.Errorf("verify called %d times, want 1", verifyCalls)
	}
	if time.Since(a.loverslab.verifiedAt) > time.Minute {
		t.Errorf("verifiedAt = %v, want it refreshed to roughly now", a.loverslab.verifiedAt)
	}
}

func TestEnsureLoversLabSessionFallsBackToTheSavedSessionWhenTheLiveClientFailsVerification(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	a.loverslab.verifiedAt = time.Now().Add(-sessionRecheckInterval - time.Minute)

	// The in-memory client fails verification exactly once (simulating a session
	// revoked server-side); the freshly re-imported saved session succeeds.
	failedOnce := false
	a.loverslab.verify = func(ctx context.Context, c *loverslab.Client) (bool, error) {
		if !failedOnce {
			failedOnce = true
			return false, nil
		}
		return true, nil
	}

	client, err := a.ensureLoversLabSession(context.Background())
	if err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if client == nil || a.loverslab.client == nil {
		t.Error("expected a freshly re-imported client to be cached")
	}
}

func TestEnsureLoversLabSessionFallsBackToAFreshLoginWhenTheSavedSessionIsUnusable(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.SaveLoversLabCredentials(testLoversLabUser, testLoversLabPass); err != nil {
		t.Fatalf("SaveLoversLabCredentials: %v", err)
	}
	// Simulate a session that expired entirely (not just gone stale in memory): drop
	// the in-memory client and corrupt the saved session, but keep username/password.
	a.loverslab.client = nil
	a.loverslab.verifiedAt = time.Time{}
	if err := a.loverslab.mgr.Save(map[string]string{"session": "not a valid session at all"}); err != nil {
		t.Fatalf("corrupting the saved session: %v", err)
	}

	loginCalls := 0
	realLogin := a.loverslab.login
	a.loverslab.login = func(ctx context.Context, auth, password string) (*loverslab.Client, error) {
		loginCalls++
		return realLogin(ctx, auth, password)
	}

	if _, err := a.ensureLoversLabSession(context.Background()); err != nil {
		t.Fatalf("ensureLoversLabSession: %v", err)
	}
	if loginCalls != 1 {
		t.Errorf("login called %d times, want 1", loginCalls)
	}
}

func TestEnsureLoversLabSessionWithNothingEverSavedFails(t *testing.T) {
	a := newLoversLabApp(t, t.TempDir())
	if _, err := a.ensureLoversLabSession(context.Background()); err == nil {
		t.Error("expected an error when nothing has ever been signed in")
	}
}
