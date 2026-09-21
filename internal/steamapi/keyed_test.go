package steamapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// sentinelKey is a recognisable key: a test that finds it in an error, a log line
// or a status has found a leak.
const sentinelKey = "SENTINELKEY0123456789ABCDEF012345"

func pointKeyedAt(t *testing.T, target string) {
	t.Helper()
	old := keyedDetailsURL
	keyedDetailsURL = target
	t.Cleanup(func() { keyedDetailsURL = old })
}

func pointFreeAt(t *testing.T, target string) {
	t.Helper()
	old := workshopDetailsURL
	workshopDetailsURL = target
	t.Cleanup(func() { workshopDetailsURL = old })
}

// The fixture is a real keyed answer for 2780180614, an unlisted item the free API
// answers "result 9" for.
func TestKeyedParsesTheRealUnlistedRecord(t *testing.T) {
	body, err := os.ReadFile("testdata/keyed_unlisted_2780180614.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotQuery url.Values
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.Query()
		w.Write(body)
	}))
	defer server.Close()
	pointKeyedAt(t, server.URL)

	res, err := GetPublishedFileDetailsWithKey(context.Background(), sentinelKey, []string{"2780180614"})
	if err != nil {
		t.Fatalf("GetPublishedFileDetailsWithKey: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotQuery.Get("key") != sentinelKey {
		t.Errorf("key param = %q", gotQuery.Get("key"))
	}
	if gotQuery.Get("publishedfileids[0]") != "2780180614" {
		t.Errorf("publishedfileids[0] = %q", gotQuery.Get("publishedfileids[0]"))
	}
	if gotQuery.Get("includetags") != "true" {
		t.Errorf("includetags = %q", gotQuery.Get("includetags"))
	}

	d, ok := res["2780180614"]
	if !ok {
		t.Fatalf("result has no entry for the id: %+v", res)
	}
	if d.Result != 1 || d.Banned {
		t.Errorf("Result = %d, Banned = %v, want 1 and false (banned arrives as a bool)", d.Result, d.Banned)
	}
	if d.Visibility != 3 {
		t.Errorf("Visibility = %d, want 3 (unlisted)", d.Visibility)
	}
	if d.Title != "OUTDATED Cross Border Trade" {
		t.Errorf("Title = %q", d.Title)
	}
	if !strings.HasPrefix(d.Description, "This mod will not be updated for Stellaris 4.0.") {
		t.Errorf("Description = %.60q, want the file_description text", d.Description)
	}
	if d.Creator != "76561198092064326" {
		t.Errorf("Creator = %q", d.Creator)
	}
	if d.TimeUpdated != 1730223116 || d.TimeCreated != 1647486579 {
		t.Errorf("TimeUpdated = %d, TimeCreated = %d", d.TimeUpdated, d.TimeCreated)
	}
	if d.Subscriptions != 25839 || d.Favorited != 2473 || d.Views != 84577 {
		t.Errorf("Subscriptions/Favorited/Views = %d/%d/%d", d.Subscriptions, d.Favorited, d.Views)
	}
	if d.FileSize != 261157 {
		t.Errorf("FileSize = %d, want 261157 (file_size arrives as a string)", d.FileSize)
	}
	if want := []string{"Diplomacy", "Economy", "Gameplay"}; strings.Join(d.Tags, ",") != strings.Join(want, ",") {
		t.Errorf("Tags = %v, want %v", d.Tags, want)
	}
	if d.Source != SourceKey {
		t.Errorf("Source = %q, want %q", d.Source, SourceKey)
	}
}

func TestKeyedStatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header string
		check  func(t *testing.T, err error)
	}{
		{"401 is a rejected key", 401, "", func(t *testing.T, err error) {
			if !errors.Is(err, ErrKeyRejected) {
				t.Errorf("err = %v, want ErrKeyRejected", err)
			}
		}},
		{"403 is a rejected key", 403, "", func(t *testing.T, err error) {
			if !errors.Is(err, ErrKeyRejected) {
				t.Errorf("err = %v, want ErrKeyRejected", err)
			}
		}},
		{"429 is a rate limit with its Retry-After", 429, "120", func(t *testing.T, err error) {
			var rl *RateLimitedError
			if !errors.As(err, &rl) || rl.RetryAfter != 2*time.Minute {
				t.Errorf("err = %v, want RateLimitedError with 2m", err)
			}
		}},
		{"500 is a plain error", 500, "", func(t *testing.T, err error) {
			if err == nil || errors.Is(err, ErrKeyRejected) || !strings.Contains(err.Error(), "HTTP 500") {
				t.Errorf("err = %v, want a plain HTTP 500 error", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.header != "" {
					w.Header().Set("Retry-After", tc.header)
				}
				w.WriteHeader(tc.status)
				// What Steam's 401 page really says: it does not echo the key, but a
				// misbehaving proxy could.
				fmt.Fprintf(w, "<html>Access is denied. key=%s</html>", r.URL.Query().Get("key"))
			}))
			defer server.Close()
			pointKeyedAt(t, server.URL)

			_, err := GetPublishedFileDetailsWithKey(context.Background(), sentinelKey, []string{"1"})
			tc.check(t, err)
			if err != nil && strings.Contains(err.Error(), sentinelKey) {
				t.Errorf("the error leaks the key: %v", err)
			}
		})
	}
}

func TestKeyedNetworkErrorDoesNotLeakTheKey(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	pointKeyedAt(t, server.URL)
	server.Close() // nothing listens any more: the request fails with a URL-carrying error

	_, err := GetPublishedFileDetailsWithKey(context.Background(), sentinelKey, []string{"1"})
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	if strings.Contains(err.Error(), sentinelKey) || strings.Contains(err.Error(), "key=") {
		t.Errorf("the error leaks the key or the URL: %v", err)
	}
}

func TestKeyedDecodeErrorDoesNotLeakTheKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json "+sentinelKey)
	}))
	defer server.Close()
	pointKeyedAt(t, server.URL)

	_, err := GetPublishedFileDetailsWithKey(context.Background(), sentinelKey, []string{"1"})
	if err == nil || strings.Contains(err.Error(), sentinelKey) {
		t.Errorf("err = %v, want an error without the key", err)
	}
}

func TestVerifyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != sentinelKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"response":{"publishedfiledetails":[{"publishedfileid":"1780481482","result":1}]}}`)
	}))
	defer server.Close()
	pointKeyedAt(t, server.URL)

	if err := VerifyKey(context.Background(), sentinelKey); err != nil {
		t.Errorf("VerifyKey with the right key: %v", err)
	}
	if err := VerifyKey(context.Background(), "WRONG"); !errors.Is(err, ErrKeyRejected) {
		t.Errorf("VerifyKey with a wrong key = %v, want ErrKeyRejected", err)
	}
}

func TestFlexTypes(t *testing.T) {
	var got rawKeyedDetails
	err := json.Unmarshal([]byte(`{"publishedfileid":12345,"result":"1","banned":1,"visibility":"3","file_size":"261157","subscriptions":25.0,"views":null}`), &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.PublishedFileID != "12345" || got.Result != 1 || !bool(got.Banned) || got.Visibility != 3 || got.FileSize != 261157 || got.Subscriptions != 25 || got.Views != 0 {
		t.Errorf("decoded = %+v", got)
	}
}

// --- batching -----------------------------------------------------------

// idList is "1".."n" as strings.
func idList(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprint(i + 1)
	}
	return ids
}

// freeServer answers every id it is asked for as result 1 and records each
// request's ids in order.
type freeServer struct {
	mu       sync.Mutex
	requests [][]string
	failOn   map[int]bool // request numbers (1-based) that get an HTTP 500
}

func (f *freeServer) handler(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var ids []string
	for i := 0; ; i++ {
		id := r.PostForm.Get(fmt.Sprintf("publishedfileids[%d]", i))
		if id == "" {
			break
		}
		ids = append(ids, id)
	}
	f.mu.Lock()
	f.requests = append(f.requests, ids)
	n := len(f.requests)
	fail := f.failOn[n]
	f.mu.Unlock()
	if fail {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var items []string
	for _, id := range ids {
		items = append(items, fmt.Sprintf(`{"publishedfileid":%q,"result":1,"title":"mod %s"}`, id, id))
	}
	fmt.Fprintf(w, `{"response":{"result":1,"resultcount":%d,"publishedfiledetails":[%s]}}`, len(ids), strings.Join(items, ","))
}

func TestFreeAPIIsAskedInChunksOf100(t *testing.T) {
	cases := []struct {
		ids  int
		want []int // ids per request
	}{
		{1, []int{1}},
		{100, []int{100}},
		{101, []int{100, 1}},
		{250, []int{100, 100, 50}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.ids, " ids"), func(t *testing.T) {
			fs := &freeServer{}
			server := httptest.NewServer(http.HandlerFunc(fs.handler))
			defer server.Close()
			pointFreeAt(t, server.URL)

			ids := idList(tc.ids)
			res, err := GetPublishedFileDetails(context.Background(), ids)
			if err != nil {
				t.Fatalf("GetPublishedFileDetails: %v", err)
			}
			if len(res) != tc.ids {
				t.Errorf("got %d results, want %d (every chunk's answer merged)", len(res), tc.ids)
			}
			if len(fs.requests) != len(tc.want) {
				t.Fatalf("%d requests, want %d", len(fs.requests), len(tc.want))
			}
			next := 1
			for i, req := range fs.requests {
				if len(req) != tc.want[i] {
					t.Errorf("request %d had %d ids, want %d", i+1, len(req), tc.want[i])
				}
				for _, id := range req { // ids 1-100 first, then 101-200, ...: in order
					if id != fmt.Sprint(next) {
						t.Fatalf("request %d: id %s out of order, want %d", i+1, id, next)
					}
					next++
				}
			}
		})
	}
}

func TestFreeAPIKeepsWhatWorkedWhenOneChunkFails(t *testing.T) {
	fs := &freeServer{failOn: map[int]bool{2: true}}
	server := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer server.Close()
	pointFreeAt(t, server.URL)

	res, err := GetPublishedFileDetails(context.Background(), idList(250))
	if err != nil {
		t.Fatalf("a middle chunk failing is not an error while others worked: %v", err)
	}
	if len(res) != 150 {
		t.Errorf("got %d results, want 150 (chunks 1 and 3)", len(res))
	}
	if _, ok := res["101"]; ok {
		t.Error("an id from the failed chunk is present")
	}
	if _, ok := res["1"]; !ok {
		t.Error("an id from the first chunk is missing")
	}
	if _, ok := res["201"]; !ok {
		t.Error("an id from the last chunk is missing")
	}
}

func TestFreeAPIErrorsOnlyWhenEveryChunkFails(t *testing.T) {
	fs := &freeServer{failOn: map[int]bool{1: true, 2: true, 3: true}}
	server := httptest.NewServer(http.HandlerFunc(fs.handler))
	defer server.Close()
	pointFreeAt(t, server.URL)

	if _, err := GetPublishedFileDetails(context.Background(), idList(250)); err == nil {
		t.Error("want an error when every chunk failed")
	}
	if len(fs.requests) != 3 {
		t.Errorf("%d requests, want all 3 tried", len(fs.requests))
	}
}

func TestKeyedIsAskedInChunksOf200(t *testing.T) {
	cases := []struct {
		ids  int
		want []int
	}{
		{200, []int{200}},
		{201, []int{200, 1}},
		{450, []int{200, 200, 50}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.ids, " ids"), func(t *testing.T) {
			var mu sync.Mutex
			var sizes []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				n := 0
				var items []string
				for i := 0; ; i++ {
					id := q.Get(fmt.Sprintf("publishedfileids[%d]", i))
					if id == "" {
						break
					}
					n++
					items = append(items, fmt.Sprintf(`{"publishedfileid":%q,"result":1}`, id))
				}
				mu.Lock()
				sizes = append(sizes, n)
				mu.Unlock()
				fmt.Fprintf(w, `{"response":{"publishedfiledetails":[%s]}}`, strings.Join(items, ","))
			}))
			defer server.Close()
			pointKeyedAt(t, server.URL)

			res, err := GetPublishedFileDetailsWithKey(context.Background(), sentinelKey, idList(tc.ids))
			if err != nil {
				t.Fatal(err)
			}
			if len(res) != tc.ids {
				t.Errorf("got %d results, want %d", len(res), tc.ids)
			}
			if fmt.Sprint(sizes) != fmt.Sprint(tc.want) {
				t.Errorf("request sizes = %v, want %v", sizes, tc.want)
			}
		})
	}
}

func TestKeyedStopsAtARateLimitButKeepsEarlierChunks(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		var items []string
		for i := 0; ; i++ {
			id := r.URL.Query().Get(fmt.Sprintf("publishedfileids[%d]", i))
			if id == "" {
				break
			}
			items = append(items, fmt.Sprintf(`{"publishedfileid":%q,"result":1}`, id))
		}
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[%s]}}`, strings.Join(items, ","))
	}))
	defer server.Close()
	pointKeyedAt(t, server.URL)

	res, err := GetPublishedFileDetailsWithKey(context.Background(), sentinelKey, idList(600))
	var rl *RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v, want a rate limit", err)
	}
	if calls != 2 {
		t.Errorf("%d requests, want the run to stop at the 429 (2)", calls)
	}
	if len(res) != 200 {
		t.Errorf("got %d results, want the 200 the first chunk returned", len(res))
	}
}
