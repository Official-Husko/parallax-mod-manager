package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/loverslab"
	"github.com/Official-Husko/parallax-mod-manager/internal/loverslabtracking"
)

func TestCheckLoversLabUpdatesReportsAChangedDateModified(t *testing.T) {
	env := newInstallEnv(t)
	installs := map[string]loverslabtracking.Entry{
		"loverslab_1": {FileURL: "https://www.loverslab.com/files/file/1-a/", FileID: 1, Title: "Mod A", InstalledDateModified: "2026-01-01T00:00:00+0000"},
	}
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatal(err)
	}
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{DateModified: "2026-02-01T00:00:00+0000"}, nil
	}

	changes, err := env.a.CheckLoversLabUpdates(env.cfg.ID)
	if err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1: %+v", len(changes), changes)
	}
	c := changes[0]
	if c.ModID != "loverslab_1" || c.Name != "Mod A" || c.Source != "loverslab" || c.Kind != "updated" || !c.New {
		t.Errorf("change = %+v", c)
	}
}

func TestCheckLoversLabUpdatesReportsNothingWhenUnchanged(t *testing.T) {
	env := newInstallEnv(t)
	installs := map[string]loverslabtracking.Entry{
		"loverslab_1": {FileURL: "https://www.loverslab.com/files/file/1-a/", FileID: 1, Title: "Mod A", InstalledDateModified: "2026-01-01T00:00:00+0000"},
	}
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatal(err)
	}
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{DateModified: "2026-01-01T00:00:00+0000"}, nil
	}

	changes, err := env.a.CheckLoversLabUpdates(env.cfg.ID)
	if err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("got %+v, want no changes", changes)
	}
}

func TestCheckLoversLabUpdatesSkipsAnEntryWithNoBaseline(t *testing.T) {
	env := newInstallEnv(t)
	installs := map[string]loverslabtracking.Entry{
		"loverslab_1": {FileURL: "https://www.loverslab.com/files/file/1-a/", FileID: 1, Title: "Mod A", InstalledDateModified: ""},
	}
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatal(err)
	}
	getFileDetailCalled := false
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		getFileDetailCalled = true
		return loverslab.FileDetail{DateModified: "2026-02-01T00:00:00+0000"}, nil
	}

	changes, err := env.a.CheckLoversLabUpdates(env.cfg.ID)
	if err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("got %+v, want no changes (no baseline to compare against)", changes)
	}
	if getFileDetailCalled {
		t.Error("getFileDetail should not even be called when there is no baseline")
	}
}

func TestCheckLoversLabUpdatesSkipsAFailedFetchRatherThanErroring(t *testing.T) {
	env := newInstallEnv(t)
	installs := map[string]loverslabtracking.Entry{
		"loverslab_1": {FileURL: "https://www.loverslab.com/files/file/1-a/", FileID: 1, Title: "Mod A", InstalledDateModified: "2026-01-01T00:00:00+0000"},
		"loverslab_2": {FileURL: "https://www.loverslab.com/files/file/2-b/", FileID: 2, Title: "Mod B", InstalledDateModified: "2026-01-01T00:00:00+0000"},
	}
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatal(err)
	}
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		if fileURL == "https://www.loverslab.com/files/file/1-a/" {
			return loverslab.FileDetail{}, context.DeadlineExceeded
		}
		return loverslab.FileDetail{DateModified: "2026-02-01T00:00:00+0000"}, nil
	}

	changes, err := env.a.CheckLoversLabUpdates(env.cfg.ID)
	if err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}
	if len(changes) != 1 || changes[0].ModID != "loverslab_2" {
		t.Errorf("got %+v, want just Mod B's update (Mod A's failed fetch skipped, not an error)", changes)
	}
}

func TestCheckLoversLabUpdatesBackfillsAMissingThumbnailFromTheSameFetch(t *testing.T) {
	env := newInstallEnv(t)
	installs := map[string]loverslabtracking.Entry{
		"loverslab_1": {FileURL: "https://www.loverslab.com/files/file/1-a/", FileID: 1, Title: "Mod A", InstalledDateModified: "2026-01-01T00:00:00+0000"},
	}
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatal(err)
	}
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{
			DateModified: "2026-01-01T00:00:00+0000", // unchanged - no update, but the fetch itself still happened
			Screenshots:  []loverslab.Screenshot{{URL: "https://static.loverslab.com/1/full.jpg", ThumbnailURL: "https://static.loverslab.com/1/thumb.jpg"}},
		}, nil
	}

	if _, err := env.a.CheckLoversLabUpdates(env.cfg.ID); err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}

	got, err := env.a.loverslabInstalls.Load(env.cfg.ID)
	if err != nil {
		t.Fatalf("loading tracked installs: %v", err)
	}
	if got["loverslab_1"].ThumbnailURL != "https://static.loverslab.com/1/thumb.jpg" {
		t.Errorf("ThumbnailURL = %q, want it backfilled from the detail fetch", got["loverslab_1"].ThumbnailURL)
	}
}

func TestCheckLoversLabUpdatesNeverOverwritesAnAlreadyTrackedThumbnail(t *testing.T) {
	env := newInstallEnv(t)
	installs := map[string]loverslabtracking.Entry{
		"loverslab_1": {
			FileURL: "https://www.loverslab.com/files/file/1-a/", FileID: 1, Title: "Mod A",
			InstalledDateModified: "2026-01-01T00:00:00+0000", ThumbnailURL: "https://static.loverslab.com/1/original.jpg",
		},
	}
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatal(err)
	}
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		return loverslab.FileDetail{
			DateModified: "2026-01-01T00:00:00+0000",
			Screenshots:  []loverslab.Screenshot{{ThumbnailURL: "https://static.loverslab.com/1/different.jpg"}},
		}, nil
	}

	if _, err := env.a.CheckLoversLabUpdates(env.cfg.ID); err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}

	got, err := env.a.loverslabInstalls.Load(env.cfg.ID)
	if err != nil {
		t.Fatalf("loading tracked installs: %v", err)
	}
	if got["loverslab_1"].ThumbnailURL != "https://static.loverslab.com/1/original.jpg" {
		t.Errorf("ThumbnailURL = %q, want the original left untouched", got["loverslab_1"].ThumbnailURL)
	}
}

// TestCheckLoversLabUpdatesFetchesConcurrentlyNotOneAtATime is the real bug this
// covers: checking a dozen-plus mods installed from LoversLab one at a time (this
// function's own previous behavior) turns into several real seconds of sequential
// page fetches every time it runs (on startup, and every few hours) - confirmed a
// real, separate contributor to "browsing feels laggy", not just slow in the
// abstract, since it competes with whatever the person is doing in Browse right
// then for the same site. This installs 8 mods, each taking 50ms to "fetch", and
// asserts the whole check finishes in well under 8 x 50ms - only possible with
// real concurrency, not just because the individual fetches happen to be fast.
func TestCheckLoversLabUpdatesFetchesConcurrentlyNotOneAtATime(t *testing.T) {
	env := newInstallEnv(t)
	const modCount = 8
	installs := map[string]loverslabtracking.Entry{}
	for i := 0; i < modCount; i++ {
		modID := fmt.Sprintf("loverslab_%d", i)
		installs[modID] = loverslabtracking.Entry{
			FileURL: fmt.Sprintf("https://www.loverslab.com/files/file/%d-mod/", i), FileID: i,
			Title: fmt.Sprintf("Mod %d", i), InstalledDateModified: "2026-01-01T00:00:00+0000",
		}
	}
	if err := env.a.loverslabInstalls.Save(env.cfg.ID, installs); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	env.a.loverslab.getFileDetail = func(ctx context.Context, client *loverslab.Client, fileURL string) (loverslab.FileDetail, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return loverslab.FileDetail{DateModified: "2026-01-01T00:00:00+0000"}, nil
	}

	start := time.Now()
	if _, err := env.a.CheckLoversLabUpdates(env.cfg.ID); err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}
	elapsed := time.Since(start)

	if got := calls.Load(); got != modCount {
		t.Fatalf("getFileDetail was called %d times, want %d (one per installed mod)", got, modCount)
	}
	if elapsed >= modCount*50*time.Millisecond {
		t.Errorf("took %v for %d mods at 50ms each - looks sequential, not concurrent", elapsed, modCount)
	}
}

func TestCheckLoversLabUpdatesWithNothingInstalledReportsNothing(t *testing.T) {
	env := newInstallEnv(t)
	changes, err := env.a.CheckLoversLabUpdates(env.cfg.ID)
	if err != nil {
		t.Fatalf("CheckLoversLabUpdates: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("got %+v, want none", changes)
	}
}
