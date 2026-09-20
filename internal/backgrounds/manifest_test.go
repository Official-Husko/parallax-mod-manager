package backgrounds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

const sampleTree = `{"sha":"x","truncated":false,"tree":[
	{"path":"g1","type":"tree"},
	{"path":"g1/b.jpg","type":"blob","size":300},
	{"path":"g1/a.png","type":"blob","size":100},
	{"path":"g1/readme.md","type":"blob","size":5},
	{"path":"g1/deeper/c.png","type":"blob","size":9},
	{"path":"g2/z.webp","type":"blob","size":50},
	{"path":"../evil/a.png","type":"blob","size":1},
	{"path":"g3/../a.png","type":"blob","size":1},
	{"path":"top.png","type":"blob","size":7}
]}`

var testSource = Source{Repo: "o/r", Branch: "main", Path: "game_media/backgrounds"}

func githubStub(t *testing.T, handler http.HandlerFunc) Fetcher {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return Fetcher{APIBase: server.URL, RawBase: server.URL, UserAgent: "test"}
}

func TestFetchParsesPacksAndSizes(t *testing.T) {
	var gotPath, gotUA, gotAccept string
	f := githubStub(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA, gotAccept = r.URL.RequestURI(), r.Header.Get("User-Agent"), r.Header.Get("Accept")
		w.Header().Set("ETag", `"abc"`)
		w.Write([]byte(sampleTree))
	})
	m, etag, notModified, err := f.Fetch(context.Background(), testSource, "")
	if err != nil || notModified || etag != `"abc"` {
		t.Fatalf("err = %v, notModified = %v, etag = %q", err, notModified, etag)
	}
	if gotPath != "/repos/o/r/git/trees/main:game_media/backgrounds?recursive=1" || gotUA != "test" || gotAccept != "application/vnd.github+json" {
		t.Errorf("request = %q, UA %q, Accept %q", gotPath, gotUA, gotAccept)
	}
	if len(m.Packs) != 2 {
		t.Fatalf("packs = %+v, want g1 and g2 only (deeper folders, other files, odd paths dropped)", m.Packs)
	}
	g1, _ := m.Pack("g1")
	if g1.Bytes != 400 || len(g1.Files) != 2 || g1.Files[0] != (File{"a.png", 100}) || g1.Files[1] != (File{"b.jpg", 300}) {
		t.Errorf("g1 = %+v, want a.png and b.jpg sorted, 400 bytes", g1)
	}
	if g2, ok := m.Pack("g2"); !ok || g2.Bytes != 50 {
		t.Errorf("g2 = %+v", g2)
	}
	if _, ok := m.Pack("g3"); ok {
		t.Error("g3 has nothing valid and must not appear")
	}
}

func TestFetchSendsTheETagAndSurvivesNotModified(t *testing.T) {
	var sent string
	f := githubStub(t, func(w http.ResponseWriter, r *http.Request) {
		sent = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	})
	_, etag, notModified, err := f.Fetch(context.Background(), testSource, `"abc"`)
	if err != nil || !notModified || sent != `"abc"` || etag != `"abc"` {
		t.Errorf("err = %v, notModified = %v, sent = %q, etag = %q", err, notModified, sent, etag)
	}
}

func TestFetchMissingFolderIsEmptyNotAnError(t *testing.T) {
	f := githubStub(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	m, _, _, err := f.Fetch(context.Background(), testSource, "")
	if err != nil || len(m.Packs) != 0 {
		t.Errorf("m = %+v, err = %v, want an empty manifest: nothing published yet", m, err)
	}
}

func TestFetchRateLimitAndOtherFailures(t *testing.T) {
	limited := githubStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	})
	if _, _, _, err := limited.Fetch(context.Background(), testSource, ""); err != ErrRateLimited {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
	for _, status := range []int{http.StatusInternalServerError, http.StatusForbidden} {
		f := githubStub(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		if _, _, _, err := f.Fetch(context.Background(), testSource, ""); err == nil || err == ErrRateLimited {
			t.Errorf("HTTP %d: err = %v, want a plain error", status, err)
		}
	}
	trunc := githubStub(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"truncated":true,"tree":[]}`)) })
	if _, _, _, err := trunc.Fetch(context.Background(), testSource, ""); err == nil {
		t.Error("a truncated listing must be an error, not a silently partial one")
	}
	junk := githubStub(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`not json`)) })
	if _, _, _, err := junk.Fetch(context.Background(), testSource, ""); err == nil {
		t.Error("an unreadable listing must be an error")
	}
	if _, _, _, err := (Fetcher{}).Fetch(context.Background(), Source{Repo: "bad"}, ""); err == nil {
		t.Error("an invalid source must be refused before any request")
	}
}

func TestManifestCacheRoundTripAndDegrades(t *testing.T) {
	c := ManifestCache{Path: filepath.Join(t.TempDir(), "sub", "manifest.json")}
	if _, ok := c.Load(); ok {
		t.Error("a missing cache must load as nothing")
	}
	want := CachedManifest{ETag: `"e"`, FetchedAt: 42, Manifest: Manifest{Packs: []Pack{{GameID: "g1", Bytes: 5, Files: []File{{"a.png", 5}}}}}}
	if err := c.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok := c.Load()
	if !ok || got.ETag != `"e"` || got.FetchedAt != 42 || len(got.Manifest.Packs) != 1 || got.Manifest.Packs[0].Files[0].Name != "a.png" {
		t.Errorf("Load = %+v, %v", got, ok)
	}
	if _, ok := (ManifestCache{}).Load(); ok {
		t.Error("an unset cache path loads nothing")
	}
	if err := (ManifestCache{}).Save(want); err == nil {
		t.Error("saving with no path must be an error")
	}
}
