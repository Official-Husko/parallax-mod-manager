package steamapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// workshopDetailsURL is a var, not a const, so tests can point it at a
// local httptest.Server instead of the real Steam endpoint.
var workshopDetailsURL = "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"

// PublishedFileDetails is one Steam Workshop item's real metadata, for the
// mod detail panel's Changes tab - confirmed against a real request against
// the live endpoint (see docs/steam-web-api.md).
type PublishedFileDetails struct {
	ID string
	// Result is Steam's own per-item status (1 = success); a banned,
	// deleted, or private item still gets an entry here with a non-1
	// Result rather than being silently dropped, so a caller can tell
	// "fetched, but Steam says this one's gone" from "never asked about
	// this one at all".
	Result int
	// Banned is true when Steam's moderators removed the item: it is still
	// looked up successfully (Result 1) but is no longer available.
	Banned bool
	// Visibility is 0 public, 1 friends-only, 2 private, 3 unlisted. An unlisted
	// item is reachable by its link and works fine in a game, but the free API
	// answers "not found" for it - only the keyed API returns it (see
	// GetPublishedFileDetailsWithKey).
	Visibility  int
	Title       string
	Description string
	PreviewURL  string
	// Creator is the uploader's SteamID64 - kept as a string, matching the
	// source JSON, rather than parsed into a Go int64: a real SteamID64
	// (e.g. "76561197989629849") routinely exceeds JS's 2^53 safe-integer
	// range, the same reason internal/library never crosses a raw 64-bit
	// value over the Wails boundary either (see its package doc comment).
	Creator       string
	TimeCreated   int64 // Unix seconds
	TimeUpdated   int64 // Unix seconds
	Subscriptions int
	Favorited     int
	Views         int
	FileSize      int64
	Tags          []string
	// Source says which API answered: "free" or "key". Empty on data from before
	// the field existed.
	Source string
}

// Which API an item's details came from (PublishedFileDetails.Source).
const (
	SourceFree = "free"
	SourceKey  = "key"
)

// Steam is reported to fail a free-API request with more than 100 ids, so the
// free endpoint is asked 100 at a time (ids 0-99, then 100-199, ...). The keyed
// endpoint accepts up to 213; 200 is used, with room to spare.
const (
	freeBatchSize  = 100
	keyedBatchSize = 200
)

// rawPublishedFileDetails mirrors the API's real JSON shape exactly -
// several numeric-looking fields (file_size, publishedfileid, creator) are
// actually JSON strings, and tags is a list of {"tag": "..."} objects, not
// a plain string array. Confirmed against a real response, not guessed.
type rawPublishedFileDetails struct {
	PublishedFileID string `json:"publishedfileid"`
	Result          int    `json:"result"`
	Banned          int    `json:"banned"`
	Visibility      int    `json:"visibility"`
	Creator         string `json:"creator"`
	FileSize        string `json:"file_size"`
	PreviewURL      string `json:"preview_url"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	TimeCreated     int64  `json:"time_created"`
	TimeUpdated     int64  `json:"time_updated"`
	Subscriptions   int    `json:"subscriptions"`
	Favorited       int    `json:"favorited"`
	Views           int    `json:"views"`
	Tags            []struct {
		Tag string `json:"tag"`
	} `json:"tags"`
}

type publishedFileDetailsResponse struct {
	Response struct {
		Result               int                       `json:"result"`
		ResultCount          int                       `json:"resultcount"`
		PublishedFileDetails []rawPublishedFileDetails `json:"publishedfiledetails"`
	} `json:"response"`
}

// GetPublishedFileDetails fetches every one of ids' real Workshop metadata
// from the free (no key) endpoint, 100 ids per request (freeBatchSize) - Steam
// is reported to fail above that - one request after another, the answers merged.
// Returns a map keyed by published file id; an id Steam doesn't recognize at all
// is simply absent from the result (not an error) - one Steam considers valid but
// banned/deleted/private/unlisted still gets an entry with a non-1 Result instead.
//
// One failing request does not throw away the others: what the requests that
// worked returned is returned, and the caller sees ids it asked for missing (the
// details cache asks for them again next time). An error is returned only when no
// request worked.
func GetPublishedFileDetails(ctx context.Context, ids []string) (map[string]PublishedFileDetails, error) {
	return fetchChunks(ctx, ids, freeBatchSize, getFreeChunk, nil)
}

// fetchChunks asks for ids in chunks of at most size, in order, and merges the
// answers. A chunk that fails is skipped over, unless stop says its error means
// asking more would be pointless (a rejected key, a rate limit): then it ends the
// run there, and the answers so far come back together with that error. Without
// such an error, the result is an error only when every chunk failed.
func fetchChunks(ctx context.Context, ids []string, size int, get func(context.Context, []string) (map[string]PublishedFileDetails, error), stop func(error) bool) (map[string]PublishedFileDetails, error) {
	result := make(map[string]PublishedFileDetails, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var firstErr error
	chunks, failed := 0, 0
	for start := 0; start < len(ids); start += size {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		end := min(start+size, len(ids))
		chunks++
		part, err := get(ctx, ids[start:end])
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			if stop != nil && stop(err) {
				return result, err
			}
			continue
		}
		for id, d := range part {
			result[id] = d
		}
	}
	if failed == chunks {
		return result, firstErr
	}
	return result, nil
}

// getFreeChunk is one request to the free endpoint.
func getFreeChunk(ctx context.Context, ids []string) (map[string]PublishedFileDetails, error) {
	form := url.Values{}
	form.Set("itemcount", strconv.Itoa(len(ids)))
	for i, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, workshopDetailsURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("steamapi: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steamapi: requesting published file details: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steamapi: published file details request failed: HTTP %d", resp.StatusCode)
	}

	var parsed publishedFileDetailsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("steamapi: decoding published file details: %w", err)
	}

	result := make(map[string]PublishedFileDetails, len(parsed.Response.PublishedFileDetails))
	for _, raw := range parsed.Response.PublishedFileDetails {
		fileSize, _ := strconv.ParseInt(raw.FileSize, 10, 64) // 0 if empty/unparseable - never fatal to the whole batch
		tags := make([]string, 0, len(raw.Tags))
		for _, t := range raw.Tags {
			tags = append(tags, t.Tag)
		}
		result[raw.PublishedFileID] = PublishedFileDetails{
			ID:            raw.PublishedFileID,
			Result:        raw.Result,
			Banned:        raw.Banned != 0,
			Visibility:    raw.Visibility,
			Title:         raw.Title,
			Description:   raw.Description,
			PreviewURL:    raw.PreviewURL,
			Creator:       raw.Creator,
			TimeCreated:   raw.TimeCreated,
			TimeUpdated:   raw.TimeUpdated,
			Subscriptions: raw.Subscriptions,
			Favorited:     raw.Favorited,
			Views:         raw.Views,
			FileSize:      fileSize,
			Tags:          tags,
			Source:        SourceFree,
		}
	}
	return result, nil
}
