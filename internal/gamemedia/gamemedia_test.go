package gamemedia

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestFindReturnsEmbeddedDefaultWhenNoOverride(t *testing.T) {
	embedded := fstest.MapFS{
		"logo/game-1.png": {Data: []byte("embedded-bytes")},
	}
	s := Store{Embedded: embedded}

	data, mimeType, ok := s.Find("logo", "game-1")
	if !ok {
		t.Fatal("expected a hit")
	}
	if string(data) != "embedded-bytes" {
		t.Errorf("data = %q", data)
	}
	if mimeType != "image/png" {
		t.Errorf("mimeType = %q, want image/png", mimeType)
	}
}

func TestFindPrefersOverrideOverEmbedded(t *testing.T) {
	overrideDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(overrideDir, "logo"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overrideDir, "logo", "game-1.png"), []byte("override-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	embedded := fstest.MapFS{
		"logo/game-1.png": {Data: []byte("embedded-bytes")},
	}
	s := Store{Embedded: embedded, OverrideDir: overrideDir}

	data, _, ok := s.Find("logo", "game-1")
	if !ok {
		t.Fatal("expected a hit")
	}
	if string(data) != "override-bytes" {
		t.Errorf("data = %q, want the override file's contents", data)
	}
}

func TestFindFallsBackToEmbeddedWhenOverrideMissing(t *testing.T) {
	overrideDir := t.TempDir() // empty - no override file for this game
	embedded := fstest.MapFS{
		"logo/game-1.png": {Data: []byte("embedded-bytes")},
	}
	s := Store{Embedded: embedded, OverrideDir: overrideDir}

	data, _, ok := s.Find("logo", "game-1")
	if !ok {
		t.Fatal("expected a hit")
	}
	if string(data) != "embedded-bytes" {
		t.Errorf("data = %q, want the embedded default's contents", data)
	}
}

func TestFindReturnsFalseWhenNeitherHasIt(t *testing.T) {
	s := Store{Embedded: fstest.MapFS{}, OverrideDir: t.TempDir()}

	_, _, ok := s.Find("logo", "does-not-exist")
	if ok {
		t.Error("expected ok=false when neither location has the file")
	}
}

func TestFindTriesEachExtensionInOrder(t *testing.T) {
	embedded := fstest.MapFS{
		"logo/game-1.svg": {Data: []byte("svg-bytes")},
	}
	s := Store{Embedded: embedded}

	data, mimeType, ok := s.Find("logo", "game-1")
	if !ok {
		t.Fatal("expected a hit")
	}
	if string(data) != "svg-bytes" || mimeType != "image/svg+xml" {
		t.Errorf("data = %q, mimeType = %q", data, mimeType)
	}
}

func TestFindWithNoOverrideDirConfigured(t *testing.T) {
	embedded := fstest.MapFS{"logo/game-1.png": {Data: []byte("x")}}
	s := Store{Embedded: embedded, OverrideDir: ""}

	_, _, ok := s.Find("logo", "game-1")
	if !ok {
		t.Fatal("expected a hit from the embedded default even with no OverrideDir configured")
	}
}
