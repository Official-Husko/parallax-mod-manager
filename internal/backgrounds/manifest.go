package backgrounds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
)

// Pack is one game's published images.
type Pack struct {
	GameID string
	Files  []File
	// Bytes is their total size - the "approximately how much will this download"
	// figure, straight from the repository's own listing.
	Bytes int64
}

// Manifest is everything published: one Pack per game that has images.
type Manifest struct {
	Packs []Pack
}

// Pack returns gameID's pack.
func (m Manifest) Pack(gameID string) (Pack, bool) {
	for _, p := range m.Packs {
		if p.GameID == gameID {
			return p, true
		}
	}
	return Pack{}, false
}

// Default hosts. Fetcher fields override them, which is how tests use a local
// server.
const (
	DefaultAPIBase = "https://api.github.com"
	DefaultRawBase = "https://raw.githubusercontent.com"
)

// ErrRateLimited means GitHub refused because the unauthenticated request limit
// (60 an hour per address) was used up. Callers fall back to what they already
// have; the listing is cached and revalidated with an ETag (a 304 does not count
// against the limit) precisely to stay far below it.
var ErrRateLimited = errors.New("backgrounds: GitHub's request limit was reached, try again later")

// Fetcher lists and downloads from GitHub. The zero value uses the real hosts.
type Fetcher struct {
	Client    *http.Client
	APIBase   string
	RawBase   string
	UserAgent string
}

func (f Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (f Fetcher) apiBase() string {
	if f.APIBase != "" {
		return strings.TrimRight(f.APIBase, "/")
	}
	return DefaultAPIBase
}

// Raw is the host images are downloaded from.
func (f Fetcher) Raw() string {
	if f.RawBase != "" {
		return strings.TrimRight(f.RawBase, "/")
	}
	return DefaultRawBase
}

func (f Fetcher) userAgent() string {
	if f.UserAgent != "" {
		return f.UserAgent
	}
	return "parallax-mod-manager"
}

type treeResponse struct {
	Truncated bool `json:"truncated"`
	Tree      []struct {
		Path string `json:"path"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"tree"`
}

// Fetch lists what src publishes in one request. etag is the one returned by the
// last successful fetch ("" for none): if nothing changed the answer is
// notModified, at no cost against the rate limit. A folder that does not exist yet
// is an empty manifest, not an error - the images simply have not been published.
func (f Fetcher) Fetch(ctx context.Context, src Source, etag string) (m Manifest, newETag string, notModified bool, err error) {
	if err := src.Validate(); err != nil {
		return Manifest{}, "", false, fmt.Errorf("backgrounds: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.apiBase()+src.TreePath(), nil)
	if err != nil {
		return Manifest{}, "", false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", f.userAgent())
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return Manifest{}, "", false, fmt.Errorf("backgrounds: listing the images: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return Manifest{}, etag, true, nil
	case http.StatusNotFound:
		return Manifest{}, "", false, nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == http.StatusTooManyRequests {
			return Manifest{}, "", false, ErrRateLimited
		}
		return Manifest{}, "", false, fmt.Errorf("backgrounds: listing the images was refused: HTTP %d", resp.StatusCode)
	case http.StatusOK:
	default:
		return Manifest{}, "", false, fmt.Errorf("backgrounds: listing the images failed: HTTP %d", resp.StatusCode)
	}

	var tree treeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&tree); err != nil {
		return Manifest{}, "", false, fmt.Errorf("backgrounds: reading the image listing: %w", err)
	}
	if tree.Truncated {
		return Manifest{}, "", false, errors.New("backgrounds: the image listing is too large to read in one request")
	}
	return parseTree(tree), resp.Header.Get("ETag"), false, nil
}

// parseTree keeps the images that sit exactly one folder down (<gameID>/<file>),
// dropping anything else in the repository folder: other files, deeper folders,
// names or ids that could not be stored safely.
func parseTree(tree treeResponse) Manifest {
	packs := map[string]*Pack{}
	for _, e := range tree.Tree {
		if e.Type != "blob" {
			continue
		}
		gameID, name, ok := strings.Cut(e.Path, "/")
		if !ok || !ValidGameID(gameID) || !ValidName(name) {
			continue
		}
		p := packs[gameID]
		if p == nil {
			p = &Pack{GameID: gameID}
			packs[gameID] = p
		}
		p.Files = append(p.Files, File{Name: name, Size: e.Size})
		p.Bytes += e.Size
	}
	m := Manifest{Packs: make([]Pack, 0, len(packs))}
	for _, p := range packs {
		sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Name < p.Files[j].Name })
		m.Packs = append(m.Packs, *p)
	}
	sort.Slice(m.Packs, func(i, j int) bool { return m.Packs[i].GameID < m.Packs[j].GameID })
	return m
}

// CachedManifest is a listing kept on disk, so the app can start without the
// network and a fresh-enough one needs no request at all.
type CachedManifest struct {
	// Source is where the listing came from. A cache written for a different source
	// (the folder moved, or the user pointed the app somewhere else) describes the
	// wrong place and must not be used.
	Source    Source
	ETag      string
	FetchedAt int64 // unix seconds
	Manifest  Manifest
}

// ManifestCache stores one CachedManifest as a file. Path empty disables it.
type ManifestCache struct {
	Path string
}

// Load returns the cached listing, or false when there is none or it is unreadable.
func (c ManifestCache) Load() (CachedManifest, bool) {
	if c.Path == "" {
		return CachedManifest{}, false
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return CachedManifest{}, false
	}
	var cm CachedManifest
	if err := json.Unmarshal(data, &cm); err != nil {
		return CachedManifest{}, false
	}
	return cm, true
}

// Save writes the cache atomically.
func (c ManifestCache) Save(cm CachedManifest) error {
	if c.Path == "" {
		return errors.New("backgrounds: manifest cache path not set")
	}
	_, err := atomicfile.WriteJSON(filepath.Dir(c.Path), filepath.Base(c.Path), cm)
	return err
}
