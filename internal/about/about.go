// Package about gathers what the About page shows: the running build's real
// details (version, commit, toolchain, platform) read from the binary itself,
// and the project's links, read from a small JSONC file that is embedded in
// the build and can be replaced by one in the user's config folder.
//
// Nothing here makes a network call. In particular the page's badges are
// drawn locally from these values rather than fetched as images from a badge
// service: this app's only third-party network traffic is Steam (see
// docs/steam-web-api.md), and an About page isn't a reason to add another.
package about

import (
	"net/url"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/jsonc"
)

// Link is one button on the About page.
type Link struct {
	// Icon is Font Awesome class names, e.g. "fa-brands fa-github". Only
	// well-formed ones are accepted (see validIcon).
	Icon  string
	Label string
	// URL is always an absolute http(s) URL - see ParseData.
	URL string
}

// Data is the About page's editable content, as read from the JSONC file.
type Data struct {
	// Author is the name shown in the "made by" line. Empty hides the line.
	Author string
	Links  []Link
}

// Info is everything the About page displays.
type Info struct {
	Name    string
	Version string
	// Commit is the short revision the binary was built from, empty when the
	// build carries no version-control stamp (a `go run`, a source tarball).
	Commit string
	// Dirty is true when the working tree had uncommitted changes at build
	// time.
	Dirty        bool
	GoVersion    string
	WailsVersion string
	OS           string
	Arch         string
	// ConfigDir and CacheDir are where this app keeps its settings and its
	// incremental parse cache, resolved by the caller (app.go owns real
	// paths) - empty if they couldn't be.
	ConfigDir string
	CacheDir  string
	// Games is how many games are registered.
	Games  int
	Author string
	// Links is never nil - a nil slice would marshal as JSON null and crash
	// the frontend's first .map on it.
	Links []Link
}

// Collect fills Info's build-derived fields. The caller adds the paths, the
// game count and the content from LoadData.
func Collect(name, version string) Info {
	info := Info{
		Name:      name,
		Version:   version,
		GoVersion: strings.TrimPrefix(runtime.Version(), "go"),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Links:     []Link{},
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, dep := range bi.Deps {
		if dep.Path == "github.com/wailsapp/wails/v2" {
			info.WailsVersion = strings.TrimPrefix(dep.Version, "v")
			if dep.Replace != nil && dep.Replace.Version != "" {
				info.WailsVersion = strings.TrimPrefix(dep.Replace.Version, "v")
			}
		}
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				info.Commit = s.Value[:7]
			} else {
				info.Commit = s.Value
			}
		case "vcs.modified":
			info.Dirty = s.Value == "true"
		}
	}
	return info
}

// A Font Awesome class list: "fa-" tokens only, so the file can't smuggle
// arbitrary class names (or anything else) into the page.
var validIcon = regexp.MustCompile(`^fa-[a-z0-9]+(?: fa-[a-z0-9]+(?:-[a-z0-9]+)*)*$`)

type dataFile struct {
	Author string `json:"author"`
	Links  []struct {
		Icon  string `json:"icon"`
		Label string `json:"label"`
		URL   string `json:"url"`
	} `json:"links"`
}

// ParseData parses the JSONC content file. An entry with no URL is skipped
// silently (that's how a link that hasn't been set up yet is written); an
// entry with a URL that isn't an absolute http(s) address, no label, or an
// icon that isn't a plain Font Awesome class list is skipped too, so a
// hand-edited file can never put a "javascript:" link or stray markup on the
// page. The valid entries keep their file order.
func ParseData(data []byte) (Data, error) {
	var f dataFile
	if err := jsonc.Unmarshal(data, &f); err != nil {
		return Data{}, err
	}
	out := Data{Author: strings.TrimSpace(f.Author), Links: []Link{}}
	for _, l := range f.Links {
		raw := strings.TrimSpace(l.URL)
		label := strings.TrimSpace(l.Label)
		icon := strings.TrimSpace(l.Icon)
		if raw == "" || label == "" || !validIcon.MatchString(icon) {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			continue
		}
		out.Links = append(out.Links, Link{Icon: icon, Label: label, URL: u.String()})
	}
	return out, nil
}

// LoadData returns the About content: overridePath's file if it exists and
// parses, otherwise the embedded default. A corrupt or unreadable override
// falls back to the embedded one rather than leaving the page empty, the
// same way internal/game.LoadRegistry treats its own override. overridePath
// may be empty.
func LoadData(embedded []byte, overridePath string) Data {
	if overridePath != "" {
		if data, err := os.ReadFile(overridePath); err == nil {
			if parsed, err := ParseData(data); err == nil {
				return parsed
			}
		}
	}
	parsed, err := ParseData(embedded)
	if err != nil {
		return Data{Links: []Link{}}
	}
	return parsed
}
