package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/companions/parallax-steam-helper/steamworks"
)

// withFakeClient replaces openClient for the duration of one test, restoring
// the real one afterward - the same seam companions/launcher-shim's own
// launchFunc uses for its tests.
func withFakeClient(t *testing.T, client *fakeClient, openErr error) {
	t.Helper()
	original := openClient
	openClient = func(string) (steamClient, error) {
		if openErr != nil {
			return nil, openErr
		}
		return client, nil
	}
	t.Cleanup(func() { openClient = original })
}

func decodeEvents(t *testing.T, raw []byte) []Event {
	t.Helper()
	var events []Event
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("decoding event line %q: %v", line, err)
		}
		events = append(events, e)
	}
	return events
}

func TestRunWritesADoneEventOnSuccessAndCreatesTheWorkDirWithSteamAppIDTxt(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "steam-work")
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		submitted: steamworks.SubmitItemUpdateResult{Result: steamworks.ResultOK, PublishedFileID: 555},
	}
	withFakeClient(t, client, nil)

	req := Request{LibraryPath: "unused-in-this-test", WorkDir: workDir, AppID: 281990, ItemID: 777, Visibility: "private"}
	reqData, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run(bytes.NewReader(reqData), &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	appIDData, err := os.ReadFile(filepath.Join(workDir, "steam_appid.txt"))
	if err != nil {
		t.Fatalf("steam_appid.txt was not written into WorkDir: %v", err)
	}
	if string(appIDData) != "281990" {
		t.Errorf("steam_appid.txt = %q, want \"281990\"", appIDData)
	}

	events := decodeEvents(t, out.Bytes())
	if len(events) == 0 {
		t.Fatal("run() wrote no events at all")
	}
	last := events[len(events)-1]
	if last.Stage != "done" || last.PublishedFileID != 555 {
		t.Errorf("last event = %+v, want stage=done publishedFileId=555", last)
	}
}

func TestRunWritesAnErrorEventAndReturnsAnErrorWhenSteamInitFails(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "steam-work")
	client := &fakeClient{initOK: false} // simulates Steam not running / not logged in
	withFakeClient(t, client, nil)

	req := Request{LibraryPath: "unused", WorkDir: workDir, AppID: 281990, Visibility: "private"}
	reqData, _ := json.Marshal(req)

	var out bytes.Buffer
	err := run(bytes.NewReader(reqData), &out)
	if err == nil {
		t.Fatal("want an error when SteamAPI_Init fails")
	}

	events := decodeEvents(t, out.Bytes())
	last := events[len(events)-1]
	if last.Stage != "error" || last.Message == "" {
		t.Errorf("last event = %+v, want stage=error with a non-empty message", last)
	}
}

func TestRunReportsAnErrorEventWhenOpeningTheLibraryFails(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "steam-work")
	withFakeClient(t, nil, os.ErrNotExist)

	req := Request{LibraryPath: "/does/not/exist.so", WorkDir: workDir, AppID: 281990, Visibility: "private"}
	reqData, _ := json.Marshal(req)

	var out bytes.Buffer
	if err := run(bytes.NewReader(reqData), &out); err == nil {
		t.Fatal("want an error when openClient itself fails")
	}
	events := decodeEvents(t, out.Bytes())
	if len(events) != 1 || events[0].Stage != "error" {
		t.Errorf("events = %+v, want exactly one stage=error event (never reaching publish at all)", events)
	}
}

func TestRunRejectsInvalidJSONOnStdinBeforeTouchingAnything(t *testing.T) {
	client := &fakeClient{initOK: true}
	withFakeClient(t, client, nil)

	var out bytes.Buffer
	err := run(strings.NewReader("not json"), &out)
	if err == nil {
		t.Fatal("want an error for invalid JSON on stdin")
	}
	if client.shutdownCalled {
		t.Error("Steam should never be touched at all when the request itself fails to parse")
	}
}
