package library

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a tiny but real 1x1 PNG, so imageMimeType's decision is exercised against
// actual bytes, not just a placeholder string.
var fakePNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
}

func TestModThumbnailUsesDescriptorPictureField(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "mod_a.mod", `name = "Mod A"
path = "mod_a"
picture = "thumb.jpg"
`)
	writeFile(t, modDir, filepath.Join("mod_a", "common", "x.txt"), "thing = {}")
	writeFile(t, modDir, filepath.Join("mod_a", "thumb.jpg"), string(fakePNGBytes))

	got, err := ModThumbnail(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ModThumbnail: %v", err)
	}
	wantPrefix := "data:image/jpeg;base64,"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("got %q, want prefix %q", got, wantPrefix)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, wantPrefix))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if string(decoded) != string(fakePNGBytes) {
		t.Error("decoded content does not match the real file's bytes")
	}
}

func TestModThumbnailFallsBackToThumbnailPngWithNoPictureField(t *testing.T) {
	modDir := t.TempDir()
	// writeMod's descriptor never declares "picture" - matches a real
	// Workshop mod observed on this machine that ships thumbnail.png
	// without ever declaring it.
	writeMod(t, modDir, "mod_a", "Mod A", `thing = {}`)
	writeFile(t, modDir, filepath.Join("mod_a", "thumbnail.png"), string(fakePNGBytes))

	got, err := ModThumbnail(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ModThumbnail: %v", err)
	}
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Errorf("got %q, want a data: URI for thumbnail.png", got)
	}
}

func TestModThumbnailEmptyWhenNoneExists(t *testing.T) {
	modDir := t.TempDir()
	writeMod(t, modDir, "mod_a", "Mod A", `thing = {}`)

	got, err := ModThumbnail(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ModThumbnail: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string when no thumbnail exists", got)
	}
}

func TestModThumbnailSkipsNonWebFormats(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "mod_a.mod", `name = "Mod A"
path = "mod_a"
picture = "thumb.dds"
`)
	writeFile(t, modDir, filepath.Join("mod_a", "common", "x.txt"), "thing = {}")
	writeFile(t, modDir, filepath.Join("mod_a", "thumb.dds"), "fake-dds-data")

	got, err := ModThumbnail(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ModThumbnail: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty - .dds isn't web-displayable and there's no thumbnail.png fallback here", got)
	}
}

func TestModThumbnailRejectsPathEscapingContentDir(t *testing.T) {
	modDir := t.TempDir()
	writeFile(t, modDir, "mod_a.mod", `name = "Mod A"
path = "mod_a"
picture = "../../secret.png"
`)
	writeFile(t, modDir, filepath.Join("mod_a", "common", "x.txt"), "thing = {}")
	if err := os.WriteFile(filepath.Join(modDir, "secret.png"), fakePNGBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ModThumbnail(context.Background(), testGameConfig(), Options{ModDir: modDir}, "mod_a")
	if err != nil {
		t.Fatalf("ModThumbnail: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty - a descriptor-declared picture path must never escape the mod's own content directory", got)
	}
}

func TestModThumbnailUnknownModErrors(t *testing.T) {
	modDir := t.TempDir()
	if _, err := ModThumbnail(context.Background(), testGameConfig(), Options{ModDir: modDir}, "does_not_exist"); err == nil {
		t.Fatal("expected an error for an unknown mod ID")
	}
}
