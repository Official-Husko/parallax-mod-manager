package about

import (
	"os"
	"path/filepath"
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
	if info.Links == nil {
		t.Error("Links must be a real empty slice, not nil (marshals as null)")
	}
}

func TestParseDataKeepsValidLinksInOrder(t *testing.T) {
	d, err := ParseData([]byte(`{
		// a comment
		"author": "  Someone ",
		"links": [
			{"icon": "fa-brands fa-github", "label": "GitHub", "url": "https://github.com/x/y"},
			{"icon": "fa-solid fa-bug", "label": "Issues", "url": "http://example.com/issues"},
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Author != "Someone" {
		t.Errorf("Author = %q, want it trimmed", d.Author)
	}
	if len(d.Links) != 2 || d.Links[0].Label != "GitHub" || d.Links[1].Label != "Issues" {
		t.Fatalf("links = %+v", d.Links)
	}
}

func TestParseDataDropsAnythingUnsafeOrUnfinished(t *testing.T) {
	d, err := ParseData([]byte(`{"links": [
		{"icon": "fa-brands fa-discord", "label": "Discord", "url": ""},
		{"icon": "fa-solid fa-skull", "label": "Script", "url": "javascript:alert(1)"},
		{"icon": "fa-solid fa-file", "label": "File", "url": "file:///etc/passwd"},
		{"icon": "fa-solid fa-link", "label": "No host", "url": "https://"},
		{"icon": "fa-solid fa-link", "label": "", "url": "https://example.com"},
		{"icon": "\"><script>x</script>", "label": "Bad icon", "url": "https://example.com"},
		{"icon": "bold", "label": "Not fa", "url": "https://example.com"},
		{"icon": "fa-solid fa-check", "label": "Fine", "url": "https://example.com/ok"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Links) != 1 || d.Links[0].Label != "Fine" {
		t.Errorf("links = %+v, want only the one valid entry", d.Links)
	}
	if d.Links == nil {
		t.Error("Links must never be nil")
	}
}

func TestLoadDataOverrideWinsButCorruptFallsBackToEmbedded(t *testing.T) {
	embedded := []byte(`{"author": "Embedded", "links": [{"icon": "fa-solid fa-check", "label": "E", "url": "https://e.example"}]}`)
	dir := t.TempDir()

	override := filepath.Join(dir, "about.jsonc")
	if err := os.WriteFile(override, []byte(`{"author": "Mine", "links": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadData(embedded, override); got.Author != "Mine" || len(got.Links) != 0 {
		t.Errorf("override should replace the embedded content entirely, got %+v", got)
	}

	if err := os.WriteFile(override, []byte(`{ not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadData(embedded, override); got.Author != "Embedded" || len(got.Links) != 1 {
		t.Errorf("a corrupt override must fall back to the embedded content, got %+v", got)
	}

	if got := LoadData(embedded, filepath.Join(dir, "missing.jsonc")); got.Author != "Embedded" {
		t.Errorf("a missing override must use the embedded content, got %+v", got)
	}
	if got := LoadData(embedded, ""); got.Author != "Embedded" {
		t.Errorf("no override path must use the embedded content, got %+v", got)
	}
}

func TestTheShippedAboutFileParsesAndHidesUnsetLinks(t *testing.T) {
	data, err := os.ReadFile("../../data/about.jsonc")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ParseData(data)
	if err != nil {
		t.Fatalf("data/about.jsonc doesn't parse: %v", err)
	}
	if len(d.Links) == 0 {
		t.Fatal("expected at least the GitHub link")
	}
	for _, l := range d.Links {
		if l.URL == "" || l.Label == "Discord" {
			t.Errorf("an unset link must be hidden, got %+v", l)
		}
	}
	if d.Links[0].Label != "GitHub" || !strings.HasPrefix(d.Links[0].URL, "https://github.com/") {
		t.Errorf("first link = %+v, want GitHub", d.Links[0])
	}
}
