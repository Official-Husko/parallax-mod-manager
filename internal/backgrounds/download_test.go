package backgrounds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// imageServer serves images/<game>/<name> from bodies, with hooks to break things.
type imageServer struct {
	bodies   map[string]string // "game/name" -> content
	hits     sync.Map          // "game/name" -> *int32
	failures map[string]int32  // "game/name" -> how many times to answer 500 first
	inFlight int32
	maxSeen  int32
	block    chan struct{} // when non-nil, every request waits on it (or its own cancellation)
}

func (s *imageServer) hitsFor(key string) int32 {
	v, _ := s.hits.LoadOrStore(key, new(int32))
	return atomic.LoadInt32(v.(*int32))
}

func (s *imageServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cur := atomic.AddInt32(&s.inFlight, 1)
	defer atomic.AddInt32(&s.inFlight, -1)
	for {
		seen := atomic.LoadInt32(&s.maxSeen)
		if cur <= seen || atomic.CompareAndSwapInt32(&s.maxSeen, seen, cur) {
			break
		}
	}
	key := strings.TrimPrefix(r.URL.Path, "/images/")
	v, _ := s.hits.LoadOrStore(key, new(int32))
	n := atomic.AddInt32(v.(*int32), 1)
	if s.block != nil {
		select {
		case <-s.block:
		case <-r.Context().Done():
			return
		}
	}
	if n <= s.failures[key] {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	body, ok := s.bodies[key]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write([]byte(body))
}

func newDownloader(t *testing.T, s *imageServer) (Downloader, Store) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	store := Store{Dir: t.TempDir()}
	return Downloader{
		Store:      store,
		URL:        func(game, name string) string { return server.URL + "/images/" + game + "/" + escapeName(name) },
		Attempts:   3,
		RetryDelay: time.Millisecond,
	}, store
}

func job(game string, files map[string]string) Job {
	j := Job{GameID: game}
	for name, body := range files {
		j.Files = append(j.Files, File{Name: name, Size: int64(len(body))})
	}
	return j
}

func readFile(t *testing.T, store Store, game, name string) string {
	t.Helper()
	p, _ := store.Path(game, name)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s/%s: %v", game, name, err)
	}
	return string(data)
}

func noPartFiles(t *testing.T, store Store) {
	t.Helper()
	filepath.Walk(store.Dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, partSuffix) {
			t.Errorf("leftover partial file %s", p)
		}
		return nil
	})
}

func TestRunDownloadsAndReportsProgress(t *testing.T) {
	files := map[string]string{"a b.png": strings.Repeat("A", 1000), "c.jpg": strings.Repeat("C", 2500), "d.webp": "D"}
	srv := &imageServer{bodies: map[string]string{}}
	for n, b := range files {
		srv.bodies["g1/"+n] = b
	}
	d, store := newDownloader(t, srv)

	var mu sync.Mutex
	var seen []Progress
	res, err := d.Run(context.Background(), []Job{job("g1", files)}, func(p Progress) {
		mu.Lock()
		seen = append(seen, p)
		mu.Unlock()
	})
	if err != nil || res.Downloaded != 3 || res.Failed != 0 || res.Skipped != 0 || res.Bytes != 3501 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	for n, b := range files {
		if got := readFile(t, store, "g1", n); got != b {
			t.Errorf("%s content differs", n)
		}
	}
	noPartFiles(t, store)

	first, last := seen[0], seen[len(seen)-1]
	if first.State != "running" || first.FilesTotal != 3 || first.BytesTotal != 3501 || first.BytesDone != 0 {
		t.Errorf("first = %+v", first)
	}
	if last.State != "done" || last.FilesDone != 3 || last.BytesDone != 3501 || last.Failed != 0 {
		t.Errorf("last = %+v", last)
	}
	var prevDone int64
	for _, p := range seen {
		if p.BytesDone < prevDone || p.BytesDone > p.BytesTotal {
			t.Errorf("bytes done went %d -> %d of %d", prevDone, p.BytesDone, p.BytesTotal)
		}
		prevDone = p.BytesDone
	}
	names := map[string]bool{}
	for _, p := range seen {
		if p.File != "" && p.GameID == "g1" {
			names[p.File] = true
		}
	}
	if len(names) == 0 {
		t.Error("progress never said which file it was working on")
	}
}

func TestRunSkipsWhatIsAlreadyThereAndRefetchesWrongSizes(t *testing.T) {
	srv := &imageServer{bodies: map[string]string{"g1/have.png": "12345", "g1/short.png": "abcdef", "g1/new.png": "xyz"}}
	d, store := newDownloader(t, srv)
	writeImage(t, store.Dir, "g1", "have.png", "12345") // right size: skipped
	writeImage(t, store.Dir, "g1", "short.png", "abc")  // wrong size (a broken earlier download): fetched again

	job := Job{GameID: "g1", Files: []File{{"have.png", 5}, {"short.png", 6}, {"new.png", 3}}}
	var last Progress
	res, err := d.Run(context.Background(), []Job{job}, func(p Progress) { last = p })
	if err != nil || res.Downloaded != 2 || res.Skipped != 1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if srv.hitsFor("g1/have.png") != 0 {
		t.Error("an image already on disk with the right size must not be requested")
	}
	if last.FilesTotal != 2 || last.BytesTotal != 9 || last.Skipped != 1 {
		t.Errorf("totals = %+v, want only the 2 images to fetch (9 bytes) with 1 skipped", last)
	}
	if got := readFile(t, store, "g1", "short.png"); got != "abcdef" {
		t.Errorf("short.png = %q, want it replaced", got)
	}

	// Running it again is a no-op.
	res, _ = d.Run(context.Background(), []Job{job}, nil)
	if res.Downloaded != 0 || res.Skipped != 3 {
		t.Errorf("second run = %+v, want everything skipped", res)
	}
}

func TestRunRetriesAFlakyImage(t *testing.T) {
	srv := &imageServer{bodies: map[string]string{"g1/a.png": "hello"}, failures: map[string]int32{"g1/a.png": 2}}
	d, _ := newDownloader(t, srv)
	var last Progress
	res, err := d.Run(context.Background(), []Job{{GameID: "g1", Files: []File{{"a.png", 5}}}}, func(p Progress) { last = p })
	if err != nil || res.Downloaded != 1 || res.Failed != 0 {
		t.Fatalf("res = %+v, err = %v, want success on the third try", res, err)
	}
	if srv.hitsFor("g1/a.png") != 3 {
		t.Errorf("hits = %d, want 3", srv.hitsFor("g1/a.png"))
	}
	if last.BytesDone != 5 || last.Failed != 0 {
		t.Errorf("last = %+v, want the retries not to inflate the byte count", last)
	}
}

func TestRunOneFailureDoesNotStopTheRest(t *testing.T) {
	srv := &imageServer{bodies: map[string]string{"g1/ok.png": "fine", "g1/short.png": "only-4"}}
	d, store := newDownloader(t, srv)
	jobs := []Job{{GameID: "g1", Files: []File{
		{"ok.png", 4},
		{"gone.png", 9},   // 404: not on the server
		{"short.png", 99}, // server sends 6 bytes but the listing said 99
	}}}
	var last Progress
	res, err := d.Run(context.Background(), jobs, func(p Progress) { last = p })
	if err != nil {
		t.Fatalf("err = %v, a failed image is reported, not returned", err)
	}
	if res.Downloaded != 1 || res.Failed != 2 || len(res.FailedFiles) != 2 {
		t.Errorf("res = %+v", res)
	}
	if last.State != "done" || last.Failed != 2 || last.FilesDone != 1 {
		t.Errorf("last = %+v", last)
	}
	if got := readFile(t, store, "g1", "ok.png"); got != "fine" {
		t.Errorf("ok.png = %q", got)
	}
	if _, ok := store.Path("g1", "short.png"); ok {
		if _, err := os.Stat(filepath.Join(store.Dir, "g1", "short.png")); err == nil {
			t.Error("a size-mismatched download must not be kept as if it were whole")
		}
	}
	noPartFiles(t, store)
	if last.BytesDone > last.BytesTotal {
		t.Errorf("done %d exceeds total %d", last.BytesDone, last.BytesTotal)
	}
}

func TestRunCancelStopsAndCleansUp(t *testing.T) {
	srv := &imageServer{
		bodies: map[string]string{"g1/a.png": "1234", "g1/b.png": "5678", "g1/c.png": "9012"},
		block:  make(chan struct{}),
	}
	d, store := newDownloader(t, srv)
	d.Concurrency = 2
	ctx, cancel := context.WithCancel(context.Background())

	var last Progress
	var mu sync.Mutex
	done := make(chan struct{})
	var res Result
	var err error
	go func() {
		res, err = d.Run(ctx, []Job{{GameID: "g1", Files: []File{{"a.png", 4}, {"b.png", 4}, {"c.png", 4}}}}, func(p Progress) {
			mu.Lock()
			last = p
			mu.Unlock()
		})
		close(done)
	}()
	deadline := time.After(3 * time.Second)
	for atomic.LoadInt32(&srv.inFlight) < 2 {
		select {
		case <-deadline:
			t.Fatal("downloads never started")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if last.State != "cancelled" {
		t.Errorf("last state = %q, want cancelled", last.State)
	}
	if res.Downloaded != 0 {
		t.Errorf("res = %+v, nothing had completed", res)
	}
	noPartFiles(t, store)
	if srv.hitsFor("g1/c.png") != 0 {
		t.Error("the queued third image must never have been requested after the cancel")
	}
}

func TestRunRefusesUnsafeInput(t *testing.T) {
	srv := &imageServer{bodies: map[string]string{"g1/ok.png": "fine"}}
	d, store := newDownloader(t, srv)

	res, err := d.Run(context.Background(), []Job{{GameID: "g1", Files: []File{
		{"../escape.png", 4}, {"a/b.png", 4}, {".hidden.png", 4}, {"notes.txt", 4}, {"ok.png", 4},
	}}}, nil)
	if err != nil || res.Downloaded != 1 {
		t.Fatalf("res = %+v, err = %v, want only ok.png fetched", res, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(store.Dir), "escape.png")); err == nil {
		t.Error("a name with ../ wrote outside the store")
	}
	if _, err := d.Run(context.Background(), []Job{{GameID: "../x", Files: []File{{"a.png", 1}}}}, nil); err == nil {
		t.Error("an unsafe game id must be refused outright")
	}
	if _, err := (Downloader{}).Run(context.Background(), nil, nil); err == nil {
		t.Error("an unconfigured downloader must say so")
	}
}

func TestRunRespectsConcurrency(t *testing.T) {
	bodies := map[string]string{}
	var files []File
	for i := 0; i < 12; i++ {
		name := string(rune('a'+i)) + ".png"
		bodies["g1/"+name] = "data"
		files = append(files, File{name, 4})
	}
	srv := &imageServer{bodies: bodies}
	d, _ := newDownloader(t, srv)
	d.Concurrency = 2
	res, err := d.Run(context.Background(), []Job{{GameID: "g1", Files: files}}, nil)
	if err != nil || res.Downloaded != 12 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if got := atomic.LoadInt32(&srv.maxSeen); got > 2 {
		t.Errorf("saw %d requests at once, want at most 2", got)
	}
}
