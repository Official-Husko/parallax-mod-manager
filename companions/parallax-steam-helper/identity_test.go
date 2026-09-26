package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/companions/parallax-steam-helper/steamworks"
)

func TestIdentityReportsThePersonaNameAndSteamIDOnSuccess(t *testing.T) {
	client := &fakeClient{initOK: true, appID: 281990, personaName: "Kestrel_Admiral", steamID: 76561197989629849}
	var events []Event
	err := identity(client, Request{AppID: 281990, WorkDir: "/tmp/x"}, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("identity() error = %v", err)
	}
	if !client.shutdownCalled {
		t.Error("Shutdown was not called")
	}
	if len(events) != 2 || events[0].Stage != "opening" || events[1].Stage != "done" {
		t.Fatalf("events = %+v, want [opening, done]", events)
	}
	if events[1].PersonaName != "Kestrel_Admiral" {
		t.Errorf("PersonaName = %q, want %q", events[1].PersonaName, "Kestrel_Admiral")
	}
	// A real SteamID64 routinely exceeds JS's 2^53 safe-integer range - kept
	// as a decimal string end to end for exactly that reason (see Event's
	// own doc comment), never a JSON number.
	if events[1].SteamID != "76561197989629849" {
		t.Errorf("SteamID = %q, want %q", events[1].SteamID, "76561197989629849")
	}
}

func TestIdentityReportsARealPNGEncodedAvatarWhenOneIsAvailable(t *testing.T) {
	client := &fakeClient{
		initOK: true, appID: 281990,
		avatarOK: true,
		avatar:   steamworks.Image{Width: 2, Height: 2, RGBA: []byte{255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255}},
	}
	var events []Event
	if err := identity(client, Request{AppID: 281990, WorkDir: "/tmp/x"}, func(e Event) { events = append(events, e) }); err != nil {
		t.Fatalf("identity() error = %v", err)
	}
	got := events[len(events)-1].AvatarPNG
	if got == "" {
		t.Fatal("AvatarPNG is empty, want a real base64-encoded PNG")
	}
	raw, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("AvatarPNG is not valid base64: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("AvatarPNG does not decode as a PNG: %v", err)
	}
	if cfg.Width != 2 || cfg.Height != 2 {
		t.Errorf("decoded PNG is %dx%d, want 2x2", cfg.Width, cfg.Height)
	}
}

func TestIdentityAvatarIsEmptyWhenNoneIsAvailable(t *testing.T) {
	client := &fakeClient{initOK: true, appID: 281990, avatarOK: false}
	var events []Event
	if err := identity(client, Request{AppID: 281990, WorkDir: "/tmp/x"}, func(e Event) { events = append(events, e) }); err != nil {
		t.Fatalf("identity() error = %v", err)
	}
	if got := events[len(events)-1].AvatarPNG; got != "" {
		t.Errorf("AvatarPNG = %q, want empty when the fake reports no avatar available", got)
	}
}

func TestIdentityAvatarIsEmptyWhenTheFakeReportsAnError(t *testing.T) {
	client := &fakeClient{initOK: true, appID: 281990, avatarOK: true, avatarErr: errors.New("GetImageRGBA failed")}
	var events []Event
	if err := identity(client, Request{AppID: 281990, WorkDir: "/tmp/x"}, func(e Event) { events = append(events, e) }); err != nil {
		t.Fatalf("identity() error = %v", err)
	}
	if got := events[len(events)-1].AvatarPNG; got != "" {
		t.Errorf("AvatarPNG = %q, want empty when Avatar() itself errors - never fail the whole identity request over a missing avatar", got)
	}
}

func TestIdentityFailsCleanlyWhenInitFails(t *testing.T) {
	client := &fakeClient{initOK: false}
	err := identity(client, Request{WorkDir: "/tmp/x"}, func(Event) {})
	if err == nil {
		t.Fatal("want an error when SteamAPI_Init fails")
	}
	if client.shutdownCalled {
		t.Error("Shutdown should never be called when Init itself failed")
	}
}

func TestIdentityFailsWhenTheAppIDDoesNotMatch(t *testing.T) {
	client := &fakeClient{initOK: true, appID: 999}
	err := identity(client, Request{AppID: 281990, WorkDir: "/tmp/x"}, func(Event) {})
	if err == nil {
		t.Fatal("want an error when Steam reports a different AppID than expected")
	}
}
