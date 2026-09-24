package main

import (
	"errors"
	"testing"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/companions/parallax-steam-helper/steamworks"
)

// fakeClient is a steamClient that never touches a real library - every
// method just records what it was called with and returns whatever the test
// pre-loaded, so publish's own orchestration (not steamworks itself) is what
// gets exercised here.
type fakeClient struct {
	initOK      bool
	appID       uint32
	personaName string
	created     steamworks.CreateItemResult
	createErr   error
	submitted   steamworks.SubmitItemUpdateResult
	submitErr   error

	shutdownCalled bool
	startedHandle  uint64
	setTitle       string
	setDescription string
	setVisibility  steamworks.Visibility
	setContent     string
	setPreview     string
	progressCalls  int
}

func (f *fakeClient) Init() bool          { return f.initOK }
func (f *fakeClient) Shutdown()           { f.shutdownCalled = true }
func (f *fakeClient) AppID() uint32       { return f.appID }
func (f *fakeClient) PersonaName() string { return f.personaName }
func (f *fakeClient) CreateItemAndWait(uint32, steamworks.FileType, time.Duration) (steamworks.CreateItemResult, error) {
	return f.created, f.createErr
}
func (f *fakeClient) StartItemUpdate(_ uint32, itemID uint64) uint64 {
	f.startedHandle = itemID
	return itemID // handle == itemID is fine for a fake; nothing here inspects its real shape
}
func (f *fakeClient) SetItemTitle(_ uint64, title string) bool { f.setTitle = title; return true }
func (f *fakeClient) SetItemDescription(_ uint64, d string) bool {
	f.setDescription = d
	return true
}
func (f *fakeClient) SetItemVisibility(_ uint64, v steamworks.Visibility) bool {
	f.setVisibility = v
	return true
}
func (f *fakeClient) SetItemContent(_ uint64, folder string) bool { f.setContent = folder; return true }
func (f *fakeClient) SetItemPreview(_ uint64, file string) bool   { f.setPreview = file; return true }
func (f *fakeClient) SubmitItemUpdateAndWait(_ uint64, _ string, _ time.Duration, onProgress func(steamworks.UpdateStatus, uint64, uint64)) (steamworks.SubmitItemUpdateResult, error) {
	if onProgress != nil {
		f.progressCalls++
		onProgress(steamworks.UpdateStatusUploadingContent, 50, 100)
	}
	return f.submitted, f.submitErr
}

func TestPublishRefusesAnUnknownVisibilityBeforeTouchingSteamAtAll(t *testing.T) {
	client := &fakeClient{initOK: true}
	err := publish(client, Request{Visibility: "public-ish"}, func(Event) {})
	if err == nil {
		t.Fatal("want an error for an unrecognized visibility string")
	}
	if client.shutdownCalled {
		t.Error("Init/Shutdown should never be reached when the request itself is invalid")
	}
}

func TestPublishDefaultsToPrivateWhenVisibilityIsEmpty(t *testing.T) {
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		created:   steamworks.CreateItemResult{Result: steamworks.ResultOK, PublishedFileID: 111},
		submitted: steamworks.SubmitItemUpdateResult{Result: steamworks.ResultOK, PublishedFileID: 111},
	}
	req := Request{AppID: 281990, Visibility: ""}
	if err := publish(client, req, func(Event) {}); err != nil {
		t.Fatalf("publish() error = %v", err)
	}
	if client.setVisibility != steamworks.VisibilityPrivate {
		t.Errorf("setVisibility = %v, want VisibilityPrivate as the safe default for an empty request field", client.setVisibility)
	}
}

func TestPublishFailsWhenSteamAppIDDoesNotMatchTheRequest(t *testing.T) {
	client := &fakeClient{initOK: true, appID: 999999} // steam_appid.txt didn't take effect
	req := Request{AppID: 281990, Visibility: "private"}
	err := publish(client, req, func(Event) {})
	if err == nil {
		t.Fatal("want an error when Steam's own reported AppID does not match the request")
	}
}

func TestPublishCreatesANewItemWhenItemIDIsZero(t *testing.T) {
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		created:   steamworks.CreateItemResult{Result: steamworks.ResultOK, PublishedFileID: 42},
		submitted: steamworks.SubmitItemUpdateResult{Result: steamworks.ResultOK, PublishedFileID: 42},
	}
	var events []Event
	req := Request{AppID: 281990, ItemID: 0, Title: "A mod", Visibility: "private"}
	if err := publish(client, req, func(e Event) { events = append(events, e) }); err != nil {
		t.Fatalf("publish() error = %v", err)
	}
	if client.startedHandle != 42 {
		t.Errorf("StartItemUpdate was called with itemID %d, want the freshly created item's own id 42", client.startedHandle)
	}

	var sawCreated, sawDone bool
	for _, e := range events {
		if e.Stage == "created" && e.PublishedFileID == 42 {
			sawCreated = true
		}
		if e.Stage == "done" && e.PublishedFileID == 42 {
			sawDone = true
		}
	}
	if !sawCreated {
		t.Errorf("events %+v never reported stage=created with the new item id", events)
	}
	if !sawDone {
		t.Errorf("events %+v never reported a final stage=done", events)
	}
}

func TestPublishReusesAnExistingItemIDWithoutCreatingAnother(t *testing.T) {
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		createErr: errors.New("CreateItem should never be called for an update"),
		submitted: steamworks.SubmitItemUpdateResult{Result: steamworks.ResultOK, PublishedFileID: 777},
	}
	req := Request{AppID: 281990, ItemID: 777, Visibility: "private"}
	if err := publish(client, req, func(Event) {}); err != nil {
		t.Fatalf("publish() error = %v (CreateItemAndWait must not have been called)", err)
	}
	if client.startedHandle != 777 {
		t.Errorf("StartItemUpdate called with %d, want the existing item id 777", client.startedHandle)
	}
}

func TestPublishSetsEveryProvidedFieldOnTheUpdateHandle(t *testing.T) {
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		submitted: steamworks.SubmitItemUpdateResult{Result: steamworks.ResultOK, PublishedFileID: 777},
	}
	req := Request{
		AppID:         281990,
		ItemID:        777,
		Title:         "Real Space Battles",
		Description:   "A total conversion.",
		ContentFolder: "/tmp/mod-content",
		PreviewFile:   "/tmp/preview.png",
		Visibility:    "unlisted",
	}
	if err := publish(client, req, func(Event) {}); err != nil {
		t.Fatalf("publish() error = %v", err)
	}
	if client.setTitle != req.Title {
		t.Errorf("setTitle = %q, want %q", client.setTitle, req.Title)
	}
	if client.setDescription != req.Description {
		t.Errorf("setDescription = %q, want %q", client.setDescription, req.Description)
	}
	if client.setContent != req.ContentFolder {
		t.Errorf("setContent = %q, want %q", client.setContent, req.ContentFolder)
	}
	if client.setPreview != req.PreviewFile {
		t.Errorf("setPreview = %q, want %q", client.setPreview, req.PreviewFile)
	}
	if client.setVisibility != steamworks.VisibilityUnlisted {
		t.Errorf("setVisibility = %v, want VisibilityUnlisted", client.setVisibility)
	}
}

func TestPublishReportsUploadProgressEvents(t *testing.T) {
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		submitted: steamworks.SubmitItemUpdateResult{Result: steamworks.ResultOK, PublishedFileID: 777},
	}
	var events []Event
	req := Request{AppID: 281990, ItemID: 777, Visibility: "private"}
	if err := publish(client, req, func(e Event) { events = append(events, e) }); err != nil {
		t.Fatalf("publish() error = %v", err)
	}
	var sawUploading bool
	for _, e := range events {
		if e.Stage == "uploading" && e.Processed == 50 && e.Total == 100 {
			sawUploading = true
		}
	}
	if !sawUploading {
		t.Errorf("events %+v never reported the fake's own uploading progress (50/100)", events)
	}
}

func TestPublishFailsCleanlyWhenSubmitDoesNotReportSuccess(t *testing.T) {
	// Regression case for the real, live outcome found manually: Steam can
	// resolve SubmitItemUpdate's own async call successfully while still
	// reporting a non-OK EResult inside it (EResult 9 / FileNotFound, seen
	// firsthand) - that must surface as a real error, not a silent success.
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		submitted: steamworks.SubmitItemUpdateResult{Result: steamworks.ResultFileNotFound, PublishedFileID: 777},
	}
	req := Request{AppID: 281990, ItemID: 777, Visibility: "private"}
	err := publish(client, req, func(Event) {})
	if err == nil {
		t.Fatal("want an error when SubmitItemUpdate resolves with a non-OK EResult")
	}
}

func TestPublishAlwaysShutsDownEvenAfterAFailure(t *testing.T) {
	client := &fakeClient{
		initOK:    true,
		appID:     281990,
		createErr: errors.New("boom"),
	}
	req := Request{AppID: 281990, ItemID: 0, Visibility: "private"}
	if err := publish(client, req, func(Event) {}); err == nil {
		t.Fatal("want an error from the fake's own createErr")
	}
	if !client.shutdownCalled {
		t.Error("Shutdown was not called after a mid-flow failure - would leak the Steamworks session")
	}
}
