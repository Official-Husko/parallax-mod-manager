package library

import (
	"context"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/applog"
	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
)

// cacheWarnings returns the warnings the "Cache" component has logged so far.
func cacheWarnings() []string {
	var out []string
	for _, e := range applog.Default().Entries() {
		if e.Component == "Cache" && e.Level == applog.Warn.String() {
			out = append(out, e.Message)
		}
	}
	return out
}

func TestALoadNamesTheModFileItCouldNotParseButOnlyTheFirstTime(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing_a = { cost = 1 }`)
	writeMod(t, modDir, "mod_broken", "The Broken Mod", `thing_b = { cost = `)
	opts := Options{CacheDir: t.TempDir(), ModDir: modDir, Order: conflict.LoadOrder{"mod_a", "mod_broken"}}

	applog.Default().Clear()
	if _, err := LoadGame(context.Background(), testGameConfig(), opts); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	got := cacheWarnings()
	if len(got) != 1 {
		t.Fatalf("first load: warnings = %q, want exactly one naming the broken file", got)
	}
	for _, want := range []string{"common/x.txt", "The Broken Mod"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning %q doesn't mention %q", got[0], want)
		}
	}
	if strings.Contains(got[0], "Mod A") {
		t.Errorf("warning %q names a mod that read cleanly", got[0])
	}

	// The file is still broken, but that isn't news on the next scan.
	applog.Default().Clear()
	if _, err := LoadGame(context.Background(), testGameConfig(), opts); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if got := cacheWarnings(); len(got) != 0 {
		t.Errorf("warm load repeated the warning: %q", got)
	}
}

func TestClipTextKeepsShortTextAndNeverSplitsACharacter(t *testing.T) {
	if got := clipText("short", 10); got != "short" {
		t.Errorf("clipText(short) = %q", got)
	}
	if got := clipText("exactly", 7); got != "exactly" {
		t.Errorf("text at the limit must be left alone, got %q", got)
	}
	// Multi-byte characters: 5 of them are 15 bytes but 5 characters.
	if got := clipText("日本語です", 3); got != "日本語..." {
		t.Errorf("clipText cut by bytes, not characters: %q", got)
	}
	if got := clipText(strings.Repeat("x", 500), maxProblemText); len([]rune(got)) != maxProblemText+3 {
		t.Errorf("a long reason should be cut to %d characters plus the marker, got %d", maxProblemText, len([]rune(got)))
	}
}
