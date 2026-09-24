package main

import "testing"

func TestIdentityReportsThePersonaNameOnSuccess(t *testing.T) {
	client := &fakeClient{initOK: true, appID: 281990, personaName: "Kestrel_Admiral"}
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
