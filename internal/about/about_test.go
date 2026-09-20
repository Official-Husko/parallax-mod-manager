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

func TestParseLicenceReadsTheTitleAndIdentifier(t *testing.T) {
	text := "# Example Non-Commercial License 2.0\n\n**EX-NCL-2.0**\n\nCopyright (c) 2026 Someone\n\n## 1. Purpose\n\n**NOT-AN-ID**\n"
	name, id := ParseLicence(text)
	if name != "Example Non-Commercial License 2.0" || id != "EX-NCL-2.0" {
		t.Errorf("got %q / %q", name, id)
	}
}

func TestParseLicenceStopsAtTheFirstSection(t *testing.T) {
	// A bold line further down (a defined term, say) is not the identifier.
	name, id := ParseLicence("# Title\n\n## 1. Purpose\n\n**Software**\n")
	if name != "Title" || id != "" {
		t.Errorf("got %q / %q, want the title and no identifier", name, id)
	}
}

func TestParseLicenceToleratesMissingPieces(t *testing.T) {
	if name, id := ParseLicence(""); name != "" || id != "" {
		t.Errorf("empty text gave %q / %q", name, id)
	}
	if name, id := ParseLicence("no headings here\n**ID-1**"); name != "" || id != "" {
		t.Errorf("text without a title gave %q / %q, want nothing", name, id)
	}
}
