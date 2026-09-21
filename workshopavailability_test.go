package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

func TestClassifyWorkshopKeepsOnlyTheNotableOnesInOrder(t *testing.T) {
	details := map[string]steamapi.PublishedFileDetails{
		"5":          {ID: "5", Result: 1},                         // ordinary
		"2780180614": {ID: "2780180614", Result: 1, Visibility: 3}, // what the key returns for the unlisted item
		"7":          {ID: "7", Result: 9},                         // page gone: deleted or private
		"8":          {ID: "8", Result: 9},                         // page up: unlisted
		"9":          {ID: "9", Result: 9},                         // page not checked: left out
		"10":         {ID: "10", Result: 1, Visibility: 2},         // private
		"11":         {ID: "11", Result: 10},                       // a busy Steam: left out
	}
	pages := map[string]bool{"7": false, "8": true}
	got := classifyWorkshop(details, pages)
	want := []WorkshopAvailability{
		{"10", "private", steamapi.ReasonRecord},
		{"2780180614", "unlisted", steamapi.ReasonRecord},
		{"7", "deleted", steamapi.ReasonPageGone},
		{"8", "unlisted", steamapi.ReasonPageUp},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPageLiveCacheLooksOnceThenRemembers(t *testing.T) {
	var calls atomic.Int32
	c := &pageLiveCache{exists: func(ctx context.Context, id string) (bool, error) {
		calls.Add(1)
		if id == "boom" {
			return false, errors.New("HTTP 503")
		}
		return id == "up", nil
	}}

	got := c.confirm(context.Background(), []string{"up", "down", "boom"}, false)
	if !got["up"] || got["down"] {
		t.Errorf("got = %v, want up=true and down=false", got)
	}
	if _, ok := got["boom"]; ok {
		t.Error("a page that could not be read was reported as known")
	}
	if calls.Load() != 3 {
		t.Errorf("first call looked %d times, want 3", calls.Load())
	}

	calls.Store(0)
	got = c.confirm(context.Background(), []string{"up", "down", "boom"}, false)
	if calls.Load() != 1 {
		t.Errorf("second call looked %d times, want only the unread one (1)", calls.Load())
	}
	if !got["up"] || got["down"] {
		t.Errorf("remembered answers changed: %v", got)
	}

	calls.Store(0)
	c.confirm(context.Background(), []string{"up", "down"}, true)
	if calls.Load() != 2 {
		t.Errorf("a refresh looked %d times, want both again (2)", calls.Load())
	}
}

func TestPageLiveCacheNoIDsDoesNothing(t *testing.T) {
	c := &pageLiveCache{exists: func(context.Context, string) (bool, error) {
		t.Fatal("looked at a page with nothing to look at")
		return false, nil
	}}
	if got := c.confirm(context.Background(), nil, false); len(got) != 0 {
		t.Errorf("got = %v", got)
	}
}
