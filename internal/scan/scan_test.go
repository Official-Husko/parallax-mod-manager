package scan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/game"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func writeDescriptor(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestScanClassifiesAndParses(t *testing.T) {
	modDir := t.TempDir()

	writeDescriptor(t, modDir, "my_local_mod.mod", `
name = "My Local Mod"
path = "my_local_mod"
`)
	writeDescriptor(t, modDir, "ugc_1830063425.mod", `
name = "AI Species Limit"
path = "ugc_1830063425"
remote_file_id = "1830063425"
`)
	writeDescriptor(t, modDir, "pdx_00001.mod", `
name = "Paradox Launcher Mod"
path = "pdx_00001"
`)
	// Not a descriptor - must be ignored.
	writeDescriptor(t, modDir, "readme.txt", "not a descriptor")

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected scan errors: %+v", result.Errors)
	}
	if len(result.Mods) != 3 {
		t.Fatalf("expected 3 mods, got %d: %+v", len(result.Mods), result.Mods)
	}

	byID := map[string]mod.Mod{}
	for _, m := range result.Mods {
		byID[m.ID] = m
	}

	local, ok := byID["my_local_mod"]
	if !ok {
		t.Fatalf("missing local mod, got IDs: %v", keysOf(byID))
	}
	if local.Source != mod.SourceLocal {
		t.Errorf("local mod Source = %v, want SourceLocal", local.Source)
	}
	if local.Descriptor.Name != "My Local Mod" {
		t.Errorf("local mod Name = %q", local.Descriptor.Name)
	}

	workshop, ok := byID["ugc_1830063425"]
	if !ok {
		t.Fatalf("missing workshop mod, got IDs: %v", keysOf(byID))
	}
	if workshop.Source != mod.SourceWorkshop {
		t.Errorf("workshop mod Source = %v, want SourceWorkshop", workshop.Source)
	}

	launcher, ok := byID["pdx_00001"]
	if !ok {
		t.Fatalf("missing paradox-launcher mod, got IDs: %v", keysOf(byID))
	}
	if launcher.Source != mod.SourceParadoxLauncher {
		t.Errorf("launcher mod Source = %v, want SourceParadoxLauncher", launcher.Source)
	}
}

func keysOf(m map[string]mod.Mod) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestScanEmptyModDirIsNotAnError(t *testing.T) {
	opts := Options{Game: game.Stellaris, ModDir: t.TempDir()}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 0 {
		t.Errorf("expected no mods, got %+v", result.Mods)
	}
}

func TestScanMissingModDirIsNotAnError(t *testing.T) {
	opts := Options{Game: game.Stellaris, ModDir: filepath.Join(t.TempDir(), "does-not-exist")}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 0 {
		t.Errorf("expected no mods, got %+v", result.Mods)
	}
}

func TestScanMalformedDescriptorIsNonFatal(t *testing.T) {
	modDir := t.TempDir()
	writeDescriptor(t, modDir, "good.mod", `name = "Good Mod"
path = "good"`)
	writeDescriptor(t, modDir, "bad.mod", `name = "unterminated string`)

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 || result.Mods[0].ID != "good" {
		t.Errorf("Mods = %+v, want exactly [good]", result.Mods)
	}
	if len(result.Errors) != 1 || result.Errors[0].Path != filepath.Join(modDir, "bad.mod") {
		t.Errorf("Errors = %+v", result.Errors)
	}
}

func TestScanRelativeContentPathResolvedAgainstModDir(t *testing.T) {
	modDir := t.TempDir()
	writeDescriptor(t, modDir, "relative.mod", `name = "Relative"
path = "relative_content"`)

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	result, err := Scan(context.Background(), opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Mods) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(result.Mods))
	}
	want := filepath.Join(modDir, "relative_content")
	if result.Mods[0].ContentPath != want {
		t.Errorf("ContentPath = %q, want %q", result.Mods[0].ContentPath, want)
	}
}

func TestScanContextCancellation(t *testing.T) {
	modDir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeDescriptor(t, modDir, string(rune('a'+i))+".mod", `name = "x"
path = "x"`)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{Game: game.Stellaris, ModDir: modDir}
	_, err := Scan(ctx, opts)
	if err == nil {
		t.Fatal("expected Scan to report the cancelled context")
	}
}
