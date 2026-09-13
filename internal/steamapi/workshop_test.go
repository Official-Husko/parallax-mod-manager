package steamapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// realPublishedFileDetailsResponse is byte-for-byte a real response from
// GetPublishedFileDetails/v1/ (trimmed description), confirmed via a live
// request during development - see docs/steam-web-api.md.
const realPublishedFileDetailsResponse = `{"response":{"result":1,"resultcount":1,"publishedfiledetails":[{"publishedfileid":"1780481482","result":1,"creator":"76561197989629849","creator_app_id":281990,"consumer_app_id":281990,"filename":"","file_size":"1000481","file_url":"","hcontent_file":"3251727913824850130","preview_url":"https://images.steamusercontent.com/ugc/1849297632027014653/241414117033498494E5E6DE5A03326392F9EC3C/","hcontent_preview":"1849297632027014653","title":"UI Overhaul Dynamic - Extended Topbar","description":"A submod for UI Overhaul Dynamic.","time_created":1561451540,"time_updated":1781554278,"visibility":0,"banned":0,"ban_reason":"","subscriptions":144655,"favorited":9652,"lifetime_subscriptions":257522,"lifetime_favorited":11320,"views":224286,"tags":[{"tag":"Graphics"},{"tag":"Fixes"},{"tag":"Overhaul"}]}]}}`

func TestGetPublishedFileDetailsParsesRealResponse(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want form-urlencoded", ct)
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Write([]byte(realPublishedFileDetailsResponse))
	}))
	defer server.Close()
	restoreURL := workshopDetailsURL
	workshopDetailsURL = server.URL
	defer func() { workshopDetailsURL = restoreURL }()

	result, err := GetPublishedFileDetails(context.Background(), []string{"1780481482"})
	if err != nil {
		t.Fatalf("GetPublishedFileDetails: %v", err)
	}
	if !strings.Contains(gotBody, "itemcount=1") || !strings.Contains(gotBody, "publishedfileids%5B0%5D=1780481482") {
		t.Errorf("request body = %q, missing expected form fields", gotBody)
	}

	d, ok := result["1780481482"]
	if !ok {
		t.Fatalf("result missing the requested id, got %+v", result)
	}
	if d.Title != "UI Overhaul Dynamic - Extended Topbar" {
		t.Errorf("Title = %q", d.Title)
	}
	if d.Result != 1 {
		t.Errorf("Result = %d, want 1", d.Result)
	}
	if d.FileSize != 1000481 {
		t.Errorf("FileSize = %d, want 1000481 (file_size arrives as a JSON string)", d.FileSize)
	}
	if d.TimeUpdated != 1781554278 {
		t.Errorf("TimeUpdated = %d", d.TimeUpdated)
	}
	if d.Subscriptions != 144655 {
		t.Errorf("Subscriptions = %d", d.Subscriptions)
	}
	if d.Creator != "76561197989629849" {
		t.Errorf("Creator = %q, want the real SteamID64 string, unchanged", d.Creator)
	}
	wantTags := []string{"Graphics", "Fixes", "Overhaul"}
	if len(d.Tags) != len(wantTags) {
		t.Fatalf("Tags = %+v, want %v", d.Tags, wantTags)
	}
	for i, tag := range wantTags {
		if d.Tags[i] != tag {
			t.Errorf("Tags[%d] = %q, want %q", i, d.Tags[i], tag)
		}
	}
}

func TestGetPublishedFileDetailsBatchesMultipleIDsInOneRequest(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Write([]byte(`{"response":{"result":1,"resultcount":2,"publishedfiledetails":[
			{"publishedfileid":"111","result":1,"title":"First"},
			{"publishedfileid":"222","result":1,"title":"Second"}
		]}}`))
	}))
	defer server.Close()
	restoreURL := workshopDetailsURL
	workshopDetailsURL = server.URL
	defer func() { workshopDetailsURL = restoreURL }()

	result, err := GetPublishedFileDetails(context.Background(), []string{"111", "222"})
	if err != nil {
		t.Fatalf("GetPublishedFileDetails: %v", err)
	}
	if requestCount != 1 {
		t.Errorf("requestCount = %d, want exactly 1 (all ids batched into one request)", requestCount)
	}
	if len(result) != 2 || result["111"].Title != "First" || result["222"].Title != "Second" {
		t.Errorf("result = %+v", result)
	}
}

func TestGetPublishedFileDetailsEmptyIDsMakesNoRequest(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
	}))
	defer server.Close()
	restoreURL := workshopDetailsURL
	workshopDetailsURL = server.URL
	defer func() { workshopDetailsURL = restoreURL }()

	result, err := GetPublishedFileDetails(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetPublishedFileDetails: %v", err)
	}
	if requested {
		t.Error("expected no HTTP request for an empty id list")
	}
	if result == nil {
		t.Error("result is nil, want a real empty map")
	}
}

func TestGetPublishedFileDetailsIncludesNonSuccessResultsRatherThanDropping(t *testing.T) {
	// A banned/deleted/private item still gets an entry - Steam reports it
	// with a non-1 result rather than omitting it, and this package
	// preserves that distinction instead of silently dropping it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"response":{"result":1,"resultcount":1,"publishedfiledetails":[
			{"publishedfileid":"999","result":9}
		]}}`))
	}))
	defer server.Close()
	restoreURL := workshopDetailsURL
	workshopDetailsURL = server.URL
	defer func() { workshopDetailsURL = restoreURL }()

	result, err := GetPublishedFileDetails(context.Background(), []string{"999"})
	if err != nil {
		t.Fatalf("GetPublishedFileDetails: %v", err)
	}
	d, ok := result["999"]
	if !ok {
		t.Fatal("expected an entry for the non-1-result id, not omission")
	}
	if d.Result == 1 {
		t.Error("Result should reflect the real non-1 status")
	}
}

func TestGetPublishedFileDetailsHTTPErrorSurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	restoreURL := workshopDetailsURL
	workshopDetailsURL = server.URL
	defer func() { workshopDetailsURL = restoreURL }()

	if _, err := GetPublishedFileDetails(context.Background(), []string{"1"}); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}
