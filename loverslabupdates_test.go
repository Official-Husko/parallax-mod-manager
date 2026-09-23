package main

import (
	"context"
	"testing"

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
