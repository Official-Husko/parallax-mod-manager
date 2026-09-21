package steamapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// keyedDetailsURL is a var, not a const, so tests can point it at a local
// httptest.Server instead of the real Steam endpoint.
var keyedDetailsURL = "https://api.steampowered.com/IPublishedFileService/GetDetails/v1/"

// verifyItemID is the Workshop item a key is checked against: a long-standing,
// public one (it is the fixture of this package's own tests). Any item would do -
// Steam answers a key it accepts with 200 whether or not the item exists.
const verifyItemID = "1780481482"

// maxKeyedResponseBytes bounds a keyed response: 200 items with full descriptions
// come to a few megabytes.
const maxKeyedResponseBytes = 64 << 20

// ErrKeyRejected means Steam refused the API key (HTTP 401 or 403): it is wrong,
// revoked, or not one Steam knows. Retrying with the same key will not help.
var ErrKeyRejected = errors.New("steamapi: Steam rejected the API key")

// RateLimitedError means Steam answered HTTP 429: the key's quota is used up (or
// the key is being throttled) for now.
type RateLimitedError struct {
	// RetryAfter is what Steam asked for in its Retry-After header, or 0 when it
	// did not say.
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("steamapi: Steam says this key is out of requests for now (retry after %s)", e.RetryAfter.Round(time.Second))
	}
	return "steamapi: Steam says this key is out of requests for now"
}

// flexInt reads a JSON number, a numeric string, or a bool as an integer. The
// keyed endpoint sends some numbers as strings ("file_size":"261157") and the free
// one sends others as numbers, so one type reads both.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	switch s {
	case "", "null":
		*f = 0
		return nil
	case "true":
		*f = 1
		return nil
	case "false":
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if fl, ferr := strconv.ParseFloat(s, 64); ferr == nil {
			n = int64(fl)
		} else {
			*f = 0 // an unreadable number is never fatal to the whole batch
			return nil
		}
	}
	*f = flexInt(n)
	return nil
}

// flexBool reads a JSON bool, a 0/1 number, or their string forms: "banned" is a
// bool in the keyed API and an int in the free one.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	switch strings.Trim(strings.TrimSpace(string(b)), `"`) {
	case "true", "1":
		*f = true
	default:
		*f = false
	}
	return nil
}

// flexString reads a JSON string or a bare number as a string (ids are strings on
// the wire, but nothing says they always will be).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexString(v)
		return nil
	}
	*f = flexString(s)
	return nil
}

// rawKeyedDetails mirrors one item of the keyed endpoint's answer, as a real
// record for an unlisted item showed it: the description is file_description, the
// tags carry a display_name next to the tag, banned is a bool, and file_size is a
// string.
type rawKeyedDetails struct {
	PublishedFileID flexString `json:"publishedfileid"`
	Result          flexInt    `json:"result"`
	Banned          flexBool   `json:"banned"`
	Visibility      flexInt    `json:"visibility"`
	Creator         flexString `json:"creator"`
	FileSize        flexInt    `json:"file_size"`
	PreviewURL      string     `json:"preview_url"`
	Title           string     `json:"title"`
	FileDescription string     `json:"file_description"`
	Description     string     `json:"description"`
	TimeCreated     flexInt    `json:"time_created"`
	TimeUpdated     flexInt    `json:"time_updated"`
	Subscriptions   flexInt    `json:"subscriptions"`
	Favorited       flexInt    `json:"favorited"`
	Views           flexInt    `json:"views"`
	Tags            []struct {
		Tag         string `json:"tag"`
		DisplayName string `json:"display_name"`
	} `json:"tags"`
}

type keyedDetailsResponse struct {
	Response struct {
		PublishedFileDetails []rawKeyedDetails `json:"publishedfiledetails"`
	} `json:"response"`
}

// GetPublishedFileDetailsWithKey fetches ids' Workshop metadata from the keyed
// endpoint (IPublishedFileService/GetDetails), 200 ids per request. Unlike the free
// endpoint it returns unlisted items in full, and it tells a deleted item (result
// 86) from one that was never there. The result has the same shape as
// GetPublishedFileDetails, with Source "key".
//
// A rejected key (ErrKeyRejected) or a used-up quota (*RateLimitedError) stops the
// run at once; whatever earlier chunks returned comes back with that error, so a
// caller can use it and fall back for the rest. Any other failing chunk is skipped,
// and an error is returned only when every chunk failed.
//
// The key travels in the URL, so nothing here ever puts a URL or the key into an
// error: every error is built from the status or the underlying network error
// alone, and the key is scrubbed from that too.
func GetPublishedFileDetailsWithKey(ctx context.Context, key string, ids []string) (map[string]PublishedFileDetails, error) {
	stop := func(err error) bool {
		var rl *RateLimitedError
		return errors.Is(err, ErrKeyRejected) || errors.As(err, &rl)
	}
	return fetchChunks(ctx, ids, keyedBatchSize, func(ctx context.Context, chunk []string) (map[string]PublishedFileDetails, error) {
		return getKeyedChunk(ctx, key, chunk)
	}, stop)
}

// VerifyKey checks a key against Steam with one real request. nil means Steam
// accepted it; ErrKeyRejected means it did not; anything else means the check
// could not be made (offline, Steam down, quota) and says nothing about the key.
func VerifyKey(ctx context.Context, key string) error {
	_, err := getKeyedChunk(ctx, key, []string{verifyItemID})
	return err
}

// getKeyedChunk is one request to the keyed endpoint.
func getKeyedChunk(ctx context.Context, key string, ids []string) (map[string]PublishedFileDetails, error) {
	q := url.Values{}
	q.Set("key", key)
	for i, id := range ids {
		q.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}
	q.Set("includetags", "true")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, keyedDetailsURL+"?"+q.Encode(), nil)
	if err != nil {
		// A URL that does not parse would carry the key in its message.
		return nil, errors.New("steamapi: building the keyed request failed")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steamapi: requesting keyed details: %s", redact(networkReason(err), key))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrKeyRejected
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &RateLimitedError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("steamapi: keyed details request failed: HTTP %d", resp.StatusCode)
	}

	var parsed keyedDetailsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxKeyedResponseBytes)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("steamapi: decoding keyed details: %s", redact(err.Error(), key))
	}

	result := make(map[string]PublishedFileDetails, len(parsed.Response.PublishedFileDetails))
	for _, raw := range parsed.Response.PublishedFileDetails {
		desc := raw.FileDescription
		if desc == "" {
			desc = raw.Description
		}
		tags := make([]string, 0, len(raw.Tags))
		for _, t := range raw.Tags {
			tags = append(tags, t.Tag)
		}
		id := string(raw.PublishedFileID)
		result[id] = PublishedFileDetails{
			ID:            id,
			Result:        int(raw.Result),
			Banned:        bool(raw.Banned),
			Visibility:    int(raw.Visibility),
			Title:         raw.Title,
			Description:   desc,
			PreviewURL:    raw.PreviewURL,
			Creator:       string(raw.Creator),
			TimeCreated:   int64(raw.TimeCreated),
			TimeUpdated:   int64(raw.TimeUpdated),
			Subscriptions: int(raw.Subscriptions),
			Favorited:     int(raw.Favorited),
			Views:         int(raw.Views),
			FileSize:      int64(raw.FileSize),
			Tags:          tags,
			Source:        SourceKey,
		}
	}
	return result, nil
}

// networkReason is the part of a transport error worth showing, without the URL
// the standard library wraps it in (that URL carries the key).
func networkReason(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err.Error()
	}
	return err.Error()
}

// redact removes the key from s, in the plain and the URL-escaped form.
func redact(s, key string) string {
	if key == "" {
		return s
	}
	s = strings.ReplaceAll(s, key, "[key]")
	if esc := url.QueryEscape(key); esc != key {
		s = strings.ReplaceAll(s, esc, "[key]")
	}
	return s
}

// parseRetryAfter reads a Retry-After header: a number of seconds, or a date.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
