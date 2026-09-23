package launchershim

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultAPIBase is GitHub's own REST API host - a Fetcher field overrides it for
// tests, the same way internal/backgrounds' own Fetcher does.
const DefaultAPIBase = "https://api.github.com"

// repoPath is this project's own GitHub repository, in the owner/name form the
// releases API expects.
const repoPath = "Official-Husko/parallax-mod-manager"

// Fetcher downloads the shim binary from this project's latest published GitHub
// release. The zero value talks to the real API; tests point APIBase at a local
// server instead. Unlike internal/backgrounds' own Fetcher, this one keeps no
// ETag/cache - installing or repairing the shim is a rare, user-initiated action,
// never polled, so there is nothing worth caching against GitHub's rate limit.
type Fetcher struct {
	Client    *http.Client
	APIBase   string
	UserAgent string
}

// DefaultFetcher is what Install actually uses - a package-level var so tests can
// point it at a local server instead of threading a Fetcher through every call site.
var DefaultFetcher = Fetcher{}

func (f Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (f Fetcher) apiBase() string {
	if f.APIBase != "" {
		return strings.TrimRight(f.APIBase, "/")
	}
	return DefaultAPIBase
}

func (f Fetcher) userAgent() string {
	if f.UserAgent != "" {
		return f.UserAgent
	}
	return "parallax-mod-manager"
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// FetchLatest downloads the pre-built shim binary matching entryPointName
// ("dowser" or "dowser.exe") from this project's latest published GitHub release -
// never a cached or older one, so an install always starts on the newest build.
// The release workflow (.github/workflows/release.yml) publishes
// shimBinaryName's exact output as its own standalone release asset for this
// reason, alongside the full app archives.
func (f Fetcher) FetchLatest(ctx context.Context, entryPointName string) ([]byte, error) {
	rel, err := f.latestRelease(ctx)
	if err != nil {
		return nil, err
	}
	want := shimBinaryName(entryPointName)
	for _, a := range rel.Assets {
		if a.Name == want {
			return f.downloadAsset(ctx, a.BrowserDownloadURL)
		}
	}
	return nil, fmt.Errorf("launchershim: release %s has no %s asset", rel.TagName, want)
}

func (f Fetcher) latestRelease(ctx context.Context) (release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiBase()+"/repos/"+repoPath+"/releases/latest", nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", f.userAgent())

	resp, err := f.client().Do(req)
	if err != nil {
		return release{}, fmt.Errorf("launchershim: checking the latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("launchershim: checking the latest release: HTTP %d", resp.StatusCode)
	}

	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return release{}, fmt.Errorf("launchershim: reading the latest release: %w", err)
	}
	return rel, nil
}

func (f Fetcher) downloadAsset(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent())

	resp, err := f.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("launchershim: downloading the shim binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("launchershim: downloading the shim binary: HTTP %d", resp.StatusCode)
	}
	// A real shim binary is a few MB at most - this only guards against a
	// misconfigured URL returning something unexpectedly large.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("launchershim: reading the downloaded shim binary: %w", err)
	}
	return data, nil
}
