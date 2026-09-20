package about

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestCollectReportsTheRunningBuild(t *testing.T) {
	info := Collect("Parallax Mod Manager", "1.2.3")
	if info.Name != "Parallax Mod Manager" || info.Version != "1.2.3" {
		t.Errorf("name/version = %q/%q", info.Name, info.Version)
	}
	if info.GoVersion == "" || strings.HasPrefix(info.GoVersion, "go") {
		t.Errorf("GoVersion = %q, want a bare version like 1.27.0", info.GoVersion)
	}
	if info.OS == "" || info.Arch == "" {
		t.Errorf("platform = %q/%q", info.OS, info.Arch)
	}
	if info.Author == "" {
		t.Error("the author line must be set")
	}
	if len(info.Links) == 0 {
		t.Error("Links must not be empty (and never nil - it marshals as null)")
	}
}

// The links are hardcoded, so a typo would ship - this is the check that stands
// in for the validation a data file would have needed.
func TestEveryHardcodedLinkIsWellFormed(t *testing.T) {
	icon := regexp.MustCompile(`^fa-(solid|brands|regular|duotone) fa-[a-z0-9]+(-[a-z0-9]+)*$|^fa-brands fa-[a-z0-9]+(-[a-z0-9]+)*$`)
	seen := map[string]bool{}
	for _, l := range links() {
		if l.Label == "" {
			t.Errorf("link %+v has no label", l)
		}
		if !icon.MatchString(l.Icon) {
			t.Errorf("link %q has an unusable icon %q", l.Label, l.Icon)
		}
		u, err := url.Parse(l.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			t.Errorf("link %q URL %q must be an absolute https address", l.Label, l.URL)
		}
		if seen[l.URL] {
			t.Errorf("link %q repeats %q", l.Label, l.URL)
		}
		seen[l.URL] = true
	}
	if links()[0].Label != "GitHub" || !strings.HasPrefix(links()[0].URL, "https://github.com/") {
		t.Errorf("the first link should be the GitHub repository, got %+v", links()[0])
	}
}
