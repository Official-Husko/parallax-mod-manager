package launchershim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// releaseServer stands in for GitHub's own releases API - it serves one release
// (tagName) with the given assets, plus the asset bodies themselves at whatever
// path each asset's own browser_download_url ends up pointing at.
func releaseServer(t *testing.T, tagName string, assetBodies map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	assets := make([]releaseAsset, 0, len(assetBodies))
	for name, body := range assetBodies {
		body := body
		downloadPath := "/download/" + name
		mux.HandleFunc(downloadPath, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		})
		assets = append(assets, releaseAsset{Name: name, BrowserDownloadURL: srv.URL + downloadPath})
	}

	mux.HandleFunc("/repos/"+repoPath+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{TagName: tagName, Assets: assets})
	})

	return srv
}

func TestFetchLatestDownloadsTheMatchingAsset(t *testing.T) {
	srv := releaseServer(t, "v1.2.3", map[string]string{
		"launcher-shim-linux-amd64":       "linux build containing " + shimMarker,
		"launcher-shim-windows-amd64.exe": "windows build containing " + shimMarker,
	})
	f := Fetcher{APIBase: srv.URL}

	data, err := f.FetchLatest(context.Background(), "dowser")
	if err != nil {
		t.Fatalf("FetchLatest(dowser): %v", err)
	}
	if string(data) != "linux build containing "+shimMarker {
		t.Errorf("FetchLatest(dowser) = %q", data)
	}

	data, err = f.FetchLatest(context.Background(), "dowser.exe")
	if err != nil {
		t.Fatalf("FetchLatest(dowser.exe): %v", err)
	}
	if string(data) != "windows build containing "+shimMarker {
		t.Errorf("FetchLatest(dowser.exe) = %q", data)
	}
}

func TestFetchLatestFailsWhenAssetIsMissing(t *testing.T) {
	srv := releaseServer(t, "v1.2.3", map[string]string{
		"some-other-file": "not the shim",
	})
	f := Fetcher{APIBase: srv.URL}

	if _, err := f.FetchLatest(context.Background(), "dowser"); err == nil {
		t.Fatal("expected an error when the release has no matching asset")
	}
}

func TestFetchLatestFailsOnANonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	f := Fetcher{APIBase: srv.URL}

	if _, err := f.FetchLatest(context.Background(), "dowser"); err == nil {
		t.Fatal("expected an error on a non-OK release response")
	}
}

func TestAcquireShimBytesPrefersTheDownload(t *testing.T) {
	srv := releaseServer(t, "v1.2.3", map[string]string{
		"launcher-shim-linux-amd64": "downloaded build containing " + shimMarker,
	})
	restore := swapFetcher(Fetcher{APIBase: srv.URL})
	defer restore()

	// The local fallback exists too, with different content - if the download is
	// really preferred, this file is never even read.
	localPath := filepath.Join(t.TempDir(), "launcher-shim-linux-amd64")
	if err := os.WriteFile(localPath, []byte("local build containing "+shimMarker), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := acquireShimBytes(context.Background(), "dowser", localPath)
	if err != nil {
		t.Fatalf("acquireShimBytes: %v", err)
	}
	if string(data) != "downloaded build containing "+shimMarker {
		t.Errorf("acquireShimBytes = %q, want the downloaded content, not the local file's", data)
	}
}

func TestAcquireShimBytesFallsBackToLocalWhenTheFetchFails(t *testing.T) {
	// A server that always 500s - stands in for "offline" or "GitHub is down"
	// equally well from acquireShimBytes' own point of view.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	restore := swapFetcher(Fetcher{APIBase: srv.URL})
	defer restore()

	localPath := filepath.Join(t.TempDir(), "launcher-shim-linux-amd64")
	if err := os.WriteFile(localPath, []byte(fakeShimContent), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := acquireShimBytes(context.Background(), "dowser", localPath)
	if err != nil {
		t.Fatalf("acquireShimBytes: %v", err)
	}
	if string(data) != fakeShimContent {
		t.Errorf("acquireShimBytes = %q, want the local fallback's content", data)
	}
}

func TestAcquireShimBytesFallsBackWhenTheDownloadedContentIsWrong(t *testing.T) {
	// The server answers with something that does not contain shimMarker at all -
	// a corrupted download, or a renamed/wrong asset, must be treated the same as
	// a failed fetch rather than being written over a real dowser/dowser.exe.
	srv := releaseServer(t, "v1.2.3", map[string]string{
		"launcher-shim-linux-amd64": "this is not a real shim build",
	})
	restore := swapFetcher(Fetcher{APIBase: srv.URL})
	defer restore()

	localPath := filepath.Join(t.TempDir(), "launcher-shim-linux-amd64")
	if err := os.WriteFile(localPath, []byte(fakeShimContent), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := acquireShimBytes(context.Background(), "dowser", localPath)
	if err != nil {
		t.Fatalf("acquireShimBytes: %v", err)
	}
	if string(data) != fakeShimContent {
		t.Errorf("acquireShimBytes = %q, want the local fallback's content", data)
	}
}

func TestAcquireShimBytesFailsWithBothSourcesUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	restore := swapFetcher(Fetcher{APIBase: srv.URL})
	defer restore()

	missingLocalPath := filepath.Join(t.TempDir(), "does-not-exist")

	if _, err := acquireShimBytes(context.Background(), "dowser", missingLocalPath); err == nil {
		t.Fatal("expected an error when both the fetch and the local fallback fail")
	}
}

// swapFetcher points DefaultFetcher at f for the duration of a test, and returns
// a func restoring the real, zero-value Fetcher afterwards.
func swapFetcher(f Fetcher) func() {
	prev := DefaultFetcher
	DefaultFetcher = f
	return func() { DefaultFetcher = prev }
}
