package library

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
)

func TestDetectGameNotInstalledNoMods(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	got, err := DetectGame(context.Background(), game.Stellaris)
	if err != nil {
		t.Fatalf("DetectGame: %v", err)
	}
	if got.Installed {
		t.Error("expected Installed to be false with no Steam library present")
	}
	if got.InstallPath != "" {
		t.Errorf("InstallPath = %q, want empty", got.InstallPath)
	}
	if got.ModCount != 0 {
		t.Errorf("ModCount = %d, want 0", got.ModCount)
	}
	wantModFolder := filepath.Join(home, ".local", "share", "Paradox Interactive", "Stellaris", "mod")
	if got.ModFolder != wantModFolder {
		t.Errorf("ModFolder = %q, want %q", got.ModFolder, wantModFolder)
	}
}

func TestDetectGameInstalledWithMods(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific default paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	installDir := filepath.Join(home, ".steam", "steam", "steamapps", "common", "Stellaris")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "launcher-settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	modDir := filepath.Join(home, ".local", "share", "Paradox Interactive", "Stellaris", "mod")
	writeMod(t, modDir, "test_mod", "Test Mod", `x = 1`)

	got, err := DetectGame(context.Background(), game.Stellaris)
	if err != nil {
		t.Fatalf("DetectGame: %v", err)
	}
	if !got.Installed {
		t.Error("expected Installed to be true")
	}
	if got.InstallPath != installDir {
		t.Errorf("InstallPath = %q, want %q", got.InstallPath, installDir)
	}
	if got.ModCount != 1 {
		t.Errorf("ModCount = %d, want 1", got.ModCount)
	}
}
