package backgrounds

import (
	"strings"
	"testing"
)

const goodSource = `{
	// where the images are
	"repo": "Official-Husko/parallax-mod-manager",
	"branch": "main",
	"path": "game_media/backgrounds", // trailing comma below is fine too
}`

func TestLoadSourceReadsTheBuiltInDefault(t *testing.T) {
	src, notice, err := LoadSource([]byte(goodSource), nil)
	if err != nil || notice != "" {
		t.Fatalf("err = %v, notice = %q", err, notice)
	}
	if src.Repo != "Official-Husko/parallax-mod-manager" || src.Branch != "main" || src.Path != "game_media/backgrounds" {
		t.Errorf("src = %+v", src)
	}
}

func TestLoadSourceOverrideWins(t *testing.T) {
	override := `{"repo": "someone/mirror", "branch": "art", "path": "pics"}`
	src, notice, err := LoadSource([]byte(goodSource), []byte(override))
	if err != nil || notice != "" || src.Repo != "someone/mirror" || src.Branch != "art" || src.Path != "pics" {
		t.Errorf("src = %+v, notice = %q, err = %v", src, notice, err)
	}
}

func TestLoadSourceBadOverrideFallsBackAndSaysWhy(t *testing.T) {
	for name, override := range map[string]string{
		"not json":       `{ nope`,
		"bad repo":       `{"repo": "../etc", "branch": "main", "path": "x"}`,
		"path escapes":   `{"repo": "a/b", "branch": "main", "path": "../x"}`,
		"absolute path":  `{"repo": "a/b", "branch": "main", "path": "/x"}`,
		"branch has ..":  `{"repo": "a/b", "branch": "a..b", "path": "x"}`,
		"missing fields": `{"repo": "a/b"}`,
	} {
		src, notice, err := LoadSource([]byte(goodSource), []byte(override))
		if err != nil {
			t.Errorf("%s: err = %v, want the built-in source with a notice", name, err)
			continue
		}
		if src.Repo != "Official-Husko/parallax-mod-manager" || notice == "" {
			t.Errorf("%s: src = %+v, notice = %q, want the built-in source and a notice", name, src, notice)
		}
	}
}

func TestLoadSourceInvalidBuiltInIsAnError(t *testing.T) {
	if _, _, err := LoadSource([]byte(`{"repo": "nope"}`), nil); err == nil {
		t.Error("an invalid built-in source must be an error (a build-time mistake)")
	}
}

func TestSourceURLs(t *testing.T) {
	src := Source{Repo: "o/r", Branch: "main", Path: "game_media/backgrounds"}
	if got := src.TreePath(); got != "/repos/o/r/git/trees/main:game_media/backgrounds?recursive=1" {
		t.Errorf("TreePath = %q", got)
	}
	got := src.RawURL("https://raw.example/", "01a0-x", "100 - nBRSxYy.jpg")
	want := "https://raw.example/o/r/main/game_media/backgrounds/01a0-x/100%20-%20nBRSxYy.jpg"
	if got != want {
		t.Errorf("RawURL = %q, want %q", got, want)
	}
	if u := LocalURL("g1", "a b#c.png"); u != "/backgrounds/g1/a%20b%23c.png" {
		t.Errorf("LocalURL = %q", u)
	}
}

func TestEscapeNameLeavesNothingThatCouldChangeThePath(t *testing.T) {
	for _, name := range []string{"a/b.png", "..%2f.png", "a?b.png", "a#b.png", "ünï.png", "a b.png"} {
		e := escapeName(name)
		if strings.ContainsAny(e, "/?# ") {
			t.Errorf("escapeName(%q) = %q still has a URL-significant character", name, e)
		}
	}
}
