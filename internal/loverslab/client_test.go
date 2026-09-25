package loverslab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingServer serves body for every request, counting how many actually
// arrived - the direct way to confirm getDocument's own cache really avoided
// a real second request, not just that it returned the same content (which
// a real, uncached re-fetch of unchanging content would too).
func countingServer(t *testing.T, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var count atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func TestGetDocumentCachesARepeatedFetchOfTheExactSameURL(t *testing.T) {
	srv, count := countingServer(t, "<html><body>hello</body></html>")
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	if _, err := c.getDocument(ctx, srv.URL); err != nil {
		t.Fatalf("first getDocument: %v", err)
	}
	if _, err := c.getDocument(ctx, srv.URL); err != nil {
		t.Fatalf("second getDocument: %v", err)
	}
	if got := count.Load(); got != 1 {
		t.Errorf("server received %d requests, want exactly 1 (the second fetch should have hit the cache)", got)
	}
}

// TestGetDocumentCoalescesConcurrentFetchesOfTheExactSameURL is the real bug this
// covers: opening a mod's detail view fetches its own file page three separate ways
// at once (the overview, the changelog, and the support-topic lookup behind
// comments), all racing to be the first to see the plain cache empty. Without
// single-flighting fetchBody, every one of those was a real, independent HTTP
// request for the identical page - up to 3x the network cost on every single mod
// opened, confirmed a real contributor to "browsing feels laggy", not just a
// theoretical race.
func TestGetDocumentCoalescesConcurrentFetchesOfTheExactSameURL(t *testing.T) {
	var count atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		// Long enough that every goroutine below reliably starts its own call - and
		// so sees the plain cache still empty - before this first request finishes
		// and populates it, without actually slowing the test down noticeably.
		time.Sleep(20 * time.Millisecond)
		w.Write([]byte("<html><body>hello</body></html>"))
	}))
	defer srv.Close()

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	const callers = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			if _, err := c.getDocument(ctx, srv.URL); err != nil {
				t.Errorf("getDocument: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := count.Load(); got != 1 {
		t.Errorf("server received %d requests from %d concurrent callers, want exactly 1", got, callers)
	}
}

func TestGetDocumentRefetchesOnceTheCacheHasExpired(t *testing.T) {
	srv, count := countingServer(t, "<html><body>hello</body></html>")
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	if _, err := c.getDocument(ctx, srv.URL); err != nil {
		t.Fatalf("first getDocument: %v", err)
	}
	// Simulate the cache entry having gone stale, rather than waiting out
	// the real pageCacheTTL in a test.
	c.cacheMu.Lock()
	entry := c.cache[srv.URL]
	entry.fetched = time.Now().Add(-pageCacheTTL - time.Second)
	c.cache[srv.URL] = entry
	c.cacheMu.Unlock()

	if _, err := c.getDocument(ctx, srv.URL); err != nil {
		t.Fatalf("second getDocument: %v", err)
	}
	if got := count.Load(); got != 2 {
		t.Errorf("server received %d requests, want exactly 2 (an expired entry must trigger a real re-fetch)", got)
	}
}

func TestGetDocumentTreatsDifferentURLsAsSeparateCacheEntries(t *testing.T) {
	var count atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { count.Add(1); w.Write([]byte("a")) })
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) { count.Add(1); w.Write([]byte("b")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := c.getDocument(ctx, srv.URL+"/a"); err != nil {
		t.Fatalf("getDocument /a: %v", err)
	}
	if _, err := c.getDocument(ctx, srv.URL+"/b"); err != nil {
		t.Fatalf("getDocument /b: %v", err)
	}
	if got := count.Load(); got != 2 {
		t.Errorf("server received %d requests, want 2 (two distinct URLs must never share a cache entry)", got)
	}
}

func TestInvalidateCachePrefixDropsOnlyMatchingEntries(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.cache = map[string]cachedPage{
		"https://www.loverslab.com/topic/1-x/":        {body: []byte("1"), fetched: time.Now()},
		"https://www.loverslab.com/topic/1-x/page/2/": {body: []byte("2"), fetched: time.Now()},
		"https://www.loverslab.com/topic/9-other/":    {body: []byte("9"), fetched: time.Now()},
	}
	c.invalidateCachePrefix("https://www.loverslab.com/topic/1-x/")
	if _, ok := c.cache["https://www.loverslab.com/topic/1-x/"]; ok {
		t.Error("the topic's own page 1 should have been invalidated")
	}
	if _, ok := c.cache["https://www.loverslab.com/topic/1-x/page/2/"]; ok {
		t.Error("the topic's own page 2 should have been invalidated too")
	}
	if _, ok := c.cache["https://www.loverslab.com/topic/9-other/"]; !ok {
		t.Error("an unrelated topic's cached page should have been left alone")
	}
}

// PostComment invalidating the topic's own cache is what makes "post a
// reply, then reload page 1" reliably show it instead of a stale pre-reply
// copy - see comments.go's own call to invalidateCachePrefix.
func TestPostCommentInvalidatesTheTopicsCachedPages(t *testing.T) {
	var getCount atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/topic/1-x/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getCount.Add(1)
			w.Write([]byte(`<input type="hidden" name="csrfKey" value="abc123">`))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	topicURL := srv.URL + "/topic/1-x/"

	if _, err := c.getDocument(ctx, topicURL); err != nil {
		t.Fatalf("priming the cache: %v", err)
	}
	if err := c.PostComment(ctx, topicURL, "<p>hi</p>"); err != nil {
		t.Fatalf("PostComment: %v", err)
	}
	if _, err := c.getDocument(ctx, topicURL); err != nil {
		t.Fatalf("getDocument after posting: %v", err)
	}
	// One GET to prime the cache, one more inside PostComment for its own
	// csrfKey lookup (a real request, since invalidation hadn't happened
	// yet), and a third real GET after posting because that post must have
	// invalidated the cache primed in the first step.
	if got := getCount.Load(); got != 3 {
		t.Errorf("server received %d GETs, want 3 (cache primed, csrfKey's own real fetch, then a real re-fetch after the invalidating post)", got)
	}
}
