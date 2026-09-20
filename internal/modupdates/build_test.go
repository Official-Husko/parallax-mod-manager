package modupdates

import (
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

var t0 = time.Unix(1000, 0)

func TestBuildRecordsWorkshopState(t *testing.T) {
	mods := []ModInput{
		{ID: "ugc_1", Name: "One", Source: SourceWorkshop, Version: "1.0", RemoteFileID: "1", Content: fp(2, 20, 3)},
		{ID: "ugc_2", Name: "Two", Source: SourceWorkshop, RemoteFileID: "2"},
		{ID: "ugc_3", Name: "Three", Source: SourceWorkshop, RemoteFileID: "3"},
		{ID: "local", Name: "Local", Source: "local"},
	}
	ws := Workshop{OK: true, Details: map[string]steamapi.PublishedFileDetails{
		"1": {ID: "1", Result: 1, TimeUpdated: 555},
		"2": {ID: "2", Result: 9},
		"3": {ID: "3", Result: 1, Banned: true, TimeUpdated: 7},
	}, PageLive: map[string]bool{"2": false}}
	s := Build(mods, ws, nil, t0)
	if s.TakenAt != 1000 || len(s.Mods) != 4 {
		t.Fatalf("snapshot = %+v", s)
	}
	if r := s.Mods["ugc_1"]; r.WorkshopUpdated != 555 || r.WorkshopGone || r.Version != "1.0" || r.Content != fp(2, 20, 3) {
		t.Errorf("ugc_1 = %+v", r)
	}
	if r := s.Mods["ugc_2"]; !r.WorkshopGone || r.GoneSince != 1000 {
		t.Errorf("a non-1 result with its page confirmed missing should be gone since now, got %+v", r)
	}
	if r := s.Mods["ugc_3"]; !r.WorkshopGone {
		t.Errorf("a banned item should be gone, got %+v", r)
	}
	if r := s.Mods["local"]; r.WorkshopGone || r.WorkshopUpdated != 0 || r.GoneSince != 0 {
		t.Errorf("a local mod has no Workshop state, got %+v", r)
	}
}

func TestBuildKeepsWhatWasKnownWhenSteamCouldNotBeAsked(t *testing.T) {
	prev := snap(1, map[string]Record{
		"ugc_1": {Source: SourceWorkshop, RemoteFileID: "1", WorkshopUpdated: 300},
		"ugc_2": {Source: SourceWorkshop, RemoteFileID: "2", WorkshopGone: true, GoneSince: 50},
	})
	mods := []ModInput{
		{ID: "ugc_1", Source: SourceWorkshop, RemoteFileID: "1"},
		{ID: "ugc_2", Source: SourceWorkshop, RemoteFileID: "2"},
	}
	s := Build(mods, Workshop{OK: false}, &prev, t0)
	if r := s.Mods["ugc_1"]; r.WorkshopUpdated != 300 || r.WorkshopGone {
		t.Errorf("ugc_1 = %+v, want the old date kept", r)
	}
	if r := s.Mods["ugc_2"]; !r.WorkshopGone || r.GoneSince != 50 {
		t.Errorf("ugc_2 = %+v, want still gone since 50", r)
	}
}

func TestBuildKeepsTheDateAnItemFirstWentMissing(t *testing.T) {
	prev := snap(1, map[string]Record{"ugc_1": {Source: SourceWorkshop, RemoteFileID: "1", WorkshopGone: true, GoneSince: 42}})
	mods := []ModInput{{ID: "ugc_1", Source: SourceWorkshop, RemoteFileID: "1"}}
	ws := Workshop{OK: true, Details: map[string]steamapi.PublishedFileDetails{"1": {Result: 9}}, PageLive: map[string]bool{"1": false}}
	if r := Build(mods, ws, &prev, t0).Mods["ugc_1"]; r.GoneSince != 42 {
		t.Errorf("GoneSince = %d, want 42 kept", r.GoneSince)
	}
}

func TestBuildAnItemSteamHasNoAnswerForIsNotGone(t *testing.T) {
	// The API omits ids it does not recognise at all; that is "unknown", not
	// "deleted" - only an explicit non-1 result or a ban counts.
	mods := []ModInput{{ID: "ugc_1", Source: SourceWorkshop, RemoteFileID: "1"}}
	ws := Workshop{OK: true, Details: map[string]steamapi.PublishedFileDetails{}}
	if r := Build(mods, ws, nil, t0).Mods["ugc_1"]; r.WorkshopGone {
		t.Errorf("got %+v, want not gone", r)
	}
}

// The reported case: Steam's API says "not found" (result 9, nothing else) for an
// item whose page is up - an author retitled it "OUTDATED ...". That is not a
// deletion.
func TestBuildAnApiNotFoundWhosePageIsUpIsNotDeleted(t *testing.T) {
	prev := snap(1, map[string]Record{"ugc_1": {Source: SourceWorkshop, RemoteFileID: "1", WorkshopUpdated: 300}})
	mods := []ModInput{{ID: "ugc_1", Source: SourceWorkshop, RemoteFileID: "1"}}
	ws := Workshop{
		OK:       true,
		Details:  map[string]steamapi.PublishedFileDetails{"1": {ID: "1", Result: 9}},
		PageLive: map[string]bool{"1": true},
	}
	r := Build(mods, ws, &prev, t0).Mods["ugc_1"]
	if r.WorkshopGone || r.GoneSince != 0 {
		t.Errorf("record = %+v, want alive: its page is up", r)
	}
	if r.WorkshopUpdated != 300 {
		t.Errorf("WorkshopUpdated = %d, want the last known 300 kept (the API gave no date)", r.WorkshopUpdated)
	}
}

func TestBuildNeverClaimsADeletionNothingConfirmed(t *testing.T) {
	mods := []ModInput{{ID: "ugc_1", Source: SourceWorkshop, RemoteFileID: "1"}}
	ws := Workshop{OK: true, Details: map[string]steamapi.PublishedFileDetails{"1": {Result: 9}}} // page not checked
	if r := Build(mods, ws, nil, t0).Mods["ugc_1"]; r.WorkshopGone {
		t.Errorf("record = %+v, want not gone: the page was never checked", r)
	}

	// With a confirmed earlier deletion on record, an unconfirmed answer keeps it.
	prev := snap(1, map[string]Record{"ugc_1": {Source: SourceWorkshop, RemoteFileID: "1", WorkshopGone: true, GoneSince: 9}})
	if r := Build(mods, ws, &prev, t0).Mods["ugc_1"]; !r.WorkshopGone || r.GoneSince != 9 {
		t.Errorf("record = %+v, want the confirmed deletion kept", r)
	}
}

func TestBuildAPageThatCameBackClearsAnEarlierDeletion(t *testing.T) {
	prev := snap(1, map[string]Record{"ugc_1": {Source: SourceWorkshop, RemoteFileID: "1", WorkshopGone: true, GoneSince: 9}})
	mods := []ModInput{{ID: "ugc_1", Source: SourceWorkshop, RemoteFileID: "1"}}
	ws := Workshop{OK: true, Details: map[string]steamapi.PublishedFileDetails{"1": {Result: 9}}, PageLive: map[string]bool{"1": true}}
	if r := Build(mods, ws, &prev, t0).Mods["ugc_1"]; r.WorkshopGone || r.GoneSince != 0 {
		t.Errorf("record = %+v, want alive again", r)
	}
}

func TestBuildBannedIsGoneWithoutAPageCheck(t *testing.T) {
	mods := []ModInput{{ID: "ugc_1", Source: SourceWorkshop, RemoteFileID: "1"}}
	ws := Workshop{OK: true, Details: map[string]steamapi.PublishedFileDetails{"1": {Result: 1, Banned: true}}}
	if r := Build(mods, ws, nil, t0).Mods["ugc_1"]; !r.WorkshopGone {
		t.Errorf("record = %+v, want gone: Steam flags it banned", r)
	}
}
