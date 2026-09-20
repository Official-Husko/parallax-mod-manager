// Package about gathers what the About page shows: the running build's real
// details (version, commit, toolchain, platform) read from the binary itself,
// and the project's own links and credit line, which are fixed in the code
// below - they describe the project, not a user preference, so there is
// nothing to configure or override.
//
// Nothing here makes a network call. In particular the page's badges are
// drawn locally from these values rather than fetched as images from a badge
// service: this app's only third-party network traffic is Steam (see
// docs/steam-web-api.md), and an About page isn't a reason to add another.
package about

import (
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
)

// Link is one button on the About page.
type Link struct {
	// Icon is Font Awesome class names, e.g. "fa-brands fa-github".
	Icon  string
	Label string
	// URL is an absolute https URL.
	URL string
}

// Author is the name shown in the "made by" line.
const Author = "Official-Husko"

// repoURL is the project's home. The other links hang off it.
const repoURL = "https://github.com/Official-Husko/parallax-mod-manager"

// links returns the About page's buttons, in the order they're shown. To add
// one, add a line here (icon is Font Awesome class names: brand logos are
// "fa-brands fa-<name>", everything else "fa-solid fa-<name>"); a test checks
// every entry is well-formed.
func links() []Link {
	return []Link{
		{Icon: "fa-brands fa-github", Label: "GitHub", URL: repoURL},
		{Icon: "fa-solid fa-bug", Label: "Report an issue", URL: repoURL + "/issues"},
		{Icon: "fa-solid fa-tag", Label: "Releases", URL: repoURL + "/releases"},
	}
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
	// LogDir is where the activity log file is kept, empty if file logging
	// couldn't start.
	LogDir string
	// Games is how many games are registered.
	Games  int
	Author string
	// LicenceName and LicenceID are the licence's own title and short identifier,
	// read out of its text (see ParseLicence) by the caller, which owns that text.
	LicenceName string
	LicenceID   string
	// Links is never nil - a nil slice would marshal as JSON null and crash
	// the frontend's first .map on it.
	Links []Link
}

// A note for later: sharing the activity log (see docs/log-sharing.md) is meant to
// offer, as an opt-in, a set of computer details - CPU, GPU, OS, mod counts,
// whether a patch was generated - for statistics and for investigating bugs
// faster. None of that is collected anywhere yet, on purpose; when it is, it
// belongs beside Collect, gathered only when the user asks for it.

// Collect fills Info: the build-derived fields, the author and the links. The
// caller adds the paths and the game count, which only it knows.
func Collect(name, version string) Info {
	info := Info{
		Name:      name,
		Version:   version,
		GoVersion: strings.TrimPrefix(runtime.Version(), "go"),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Author:    Author,
		Links:     links(),
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

var licenceID = regexp.MustCompile(`^\*\*([A-Za-z0-9][A-Za-z0-9.\-]*)\*\*$`)

// ParseLicence reads a licence's title and short identifier from the top of its
// markdown text: the first "# Title" line, and the bold identifier line after it
// ("**PMM-NCSL-1.0**"). Taken from the text itself, not written out again here,
// so the About page can never name a licence other than the one that ships. Either
// is empty when the text does not have it; scanning stops at the first section.
func ParseLicence(text string) (name, id string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "## "):
			return name, id
		case name == "" && strings.HasPrefix(line, "# "):
			name = strings.TrimSpace(line[2:])
		case name != "" && id == "":
			if m := licenceID.FindStringSubmatch(line); m != nil {
				return name, m[1]
			}
		}
	}
	return name, id
}
