package backgrounds

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func serveOnce(t *testing.T, store Store, method, target string) *http.Response {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next", "yes")
		w.WriteHeader(http.StatusTeapot)
	})
	h := Middleware(func() Store { return store })(next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec.Result()
}

func TestMiddlewareServesAnOfflineImage(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	writeImage(t, store.Dir, "game1", "100 - a b.jpg", "JPEGDATA")

	resp := serveOnce(t, store, http.MethodGet, LocalURL("game1", "100 - a b.jpg"))
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "JPEGDATA" {
		t.Fatalf("status %d, body %q", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Cache-Control") == "" {
		t.Errorf("headers = %v", resp.Header)
	}
	if head := serveOnce(t, store, http.MethodHead, LocalURL("game1", "100 - a b.jpg")); head.StatusCode != 200 {
		t.Errorf("HEAD status %d", head.StatusCode)
	}
}

func TestMiddlewareRefusesEverythingThatIsNotAStoredImage(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	writeImage(t, store.Dir, "game1", "ok.png", "x")
	writeImage(t, store.Dir, "game1", "secret.txt", "x")
	if err := os.WriteFile(filepath.Join(filepath.Dir(store.Dir), "outside.png"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(store.Dir, "game1", "dir.png"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{
		"/backgrounds/game1/missing.png",
		"/backgrounds/game1/secret.txt",
		"/backgrounds/game1/dir.png",
		"/backgrounds/game1/",
		"/backgrounds/game1",
		"/backgrounds/",
		"/backgrounds/../outside.png",
		"/backgrounds/game1/../../outside.png",
		"/backgrounds/game1/%2e%2e/outside.png",
		"/backgrounds/game1/..%2foutside.png",
		"/backgrounds/%2e%2e/outside.png",
		"/backgrounds/unknown/ok.png",
	} {
		if resp := serveOnce(t, store, http.MethodGet, target); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, resp.StatusCode)
		}
	}
	if resp := serveOnce(t, store, http.MethodPost, "/backgrounds/game1/ok.png"); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", resp.StatusCode)
	}
	if resp := serveOnce(t, Store{}, http.MethodGet, "/backgrounds/game1/ok.png"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("no store = %d, want 404", resp.StatusCode)
	}
}

func TestMiddlewarePassesOtherRequestsOn(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	for _, target := range []string{"/", "/index.html", "/assets/app.js", "/backgroundsX/a.png", "/background/a.png"} {
		resp := serveOnce(t, store, http.MethodGet, target)
		if resp.StatusCode != http.StatusTeapot || resp.Header.Get("X-Next") != "yes" {
			t.Errorf("GET %s did not reach the next handler: %d", target, resp.StatusCode)
		}
	}
}
