package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabmeta"
)

// signIn makes a as if it's already signed in (see newInstallEnv's own
// identical trick in loverslabinstall_test.go), so ensureLoversLabSession
// returns straight away rather than trying to sign in for real.
func signIn(t *testing.T, a *App) {
	t.Helper()
	a.loverslab.client = fakeLoversLabSession(t)
	a.loverslab.verifiedAt = time.Now()
}

func TestWithCachedMetaFillsInACardsMissingFieldsWhenCached(t *testing.T) {
	f := loverslab.FileSummary{ID: 8719, Title: "Lustful Void", URL: "https://example.com/8719", Author: "Lithia<3", AuthorURL: "https://example.com/profile/1"}
	cached := map[int]loverslabmeta.Entry{
		8719: {AuthorAvatarURL: "https://static.loverslab.com/a.jpg", Views: 1000, Updated: "2026-09-13T20:01:52+0200", CachedAt: 1},
	}
	got := withCachedMeta(f, cached)
	if got.ID != 8719 || got.Title != "Lustful Void" || got.Author != "Lithia<3" {
		t.Errorf("the listing page's own fields must be kept as-is: %+v", got)
	}
	if got.AuthorAvatarURL != "https://static.loverslab.com/a.jpg" {
		t.Errorf("AuthorAvatarURL = %q, want the cached one", got.AuthorAvatarURL)
	}
	if got.Views != 1000 {
		t.Errorf("Views = %d, want the cached 1000", got.Views)
	}
	if got.RealUpdated != "2026-09-13T20:01:52+0200" {
		t.Errorf("RealUpdated = %q, want the cached ISO timestamp", got.RealUpdated)
	}
}

func TestWithCachedMetaLeavesTheExtrasBlankWhenNeverCached(t *testing.T) {
	f := loverslab.FileSummary{ID: 999, Title: "Never Opened"}
	got := withCachedMeta(f, map[int]loverslabmeta.Entry{8719: {Views: 1}})
	if got.AuthorAvatarURL != "" || got.Views != 0 || got.RealUpdated != "" {
		t.Errorf("a file with no cache entry should show none of these, got %+v", got)
	}
}

func TestWithCachedMetaHandlesANilCacheWithoutPanicking(t *testing.T) {
	f := loverslab.FileSummary{ID: 1, Title: "X"}
	got := withCachedMeta(f, nil)
	if got.ID != 1 || got.AuthorAvatarURL != "" {
		t.Errorf("got %+v", got)
	}
}

// LoversLabFileDetail is the real write side of the cache - opening a mod's
// own detail view is a real fetch happening for its own reason anyway, and
// cacheFileMeta piggybacks on it so a later visit to the browsing grid can
// show that file's own real avatar/views/date on its card.
func TestLoversLabFileDetailCachesTheRealFileMetaAsASideEffect(t *testing.T) {
	dir := t.TempDir()
	a := newLoversLabApp(t, dir)
	signIn(t, a)
	a.loverslabMeta = loverslabmeta.Store{Dir: filepath.Join(dir, "loverslab_meta")}
	a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{
			Title: "Lustful Void",
			Author: loverslab.FileAuthor{
				Name: "Lithia<3", URL: "https://example.com/profile/1", ImageURL: "https://static.loverslab.com/a.jpg",
			},
			Views:        1000,
			DateModified: "2026-09-13T20:01:52+0200",
		}, nil
	}

	if _, err := a.LoversLabFileDetail("https://www.loverslab.com/files/file/8719-stellaris-lustful-void/"); err != nil {
		t.Fatalf("LoversLabFileDetail: %v", err)
	}

	cached, err := a.loverslabMeta.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	entry, ok := cached[8719]
	if !ok {
		t.Fatalf("no cache entry was written for file 8719: %+v", cached)
	}
	if entry.AuthorAvatarURL != "https://static.loverslab.com/a.jpg" || entry.Views != 1000 || entry.Updated != "2026-09-13T20:01:52+0200" {
		t.Errorf("cached entry = %+v, want the real fetched detail's own fields", entry)
	}
	if entry.CachedAt == 0 {
		t.Error("CachedAt was never set")
	}
}

// A URL this app cannot parse a real file ID out of at all (never actually
// happens for a real Downloads file page, but must never panic or corrupt
// the cache either) is simply not cached - the detail fetch itself still
// succeeds and returns normally.
func TestLoversLabFileDetailWithAnUnparsableURLStillSucceedsButCachesNothing(t *testing.T) {
	dir := t.TempDir()
	a := newLoversLabApp(t, dir)
	signIn(t, a)
	a.loverslabMeta = loverslabmeta.Store{Dir: filepath.Join(dir, "loverslab_meta")}
	a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{Title: "Odd Page"}, nil
	}

	detail, err := a.LoversLabFileDetail("https://www.loverslab.com/not-a-normal-file-url/")
	if err != nil || detail.Title != "Odd Page" {
		t.Fatalf("LoversLabFileDetail = %+v, %v", detail, err)
	}
	cached, err := a.loverslabMeta.Load()
	if err != nil || len(cached) != 0 {
		t.Errorf("expected nothing cached for an unparsable URL, got %+v, %v", cached, err)
	}
}
