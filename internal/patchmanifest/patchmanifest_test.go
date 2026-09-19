package patchmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sample() Manifest {
	return Manifest{
		Generation:  3,
		GeneratedAt: 1_700_000_000,
		GameVersion: "v4.4.6",
		Mods: map[string]ModRecord{
			"mod_a": {Name: "Mod A", Version: "1.2"},
			"mod_b": {Name: "Mod B"},
		},
		Keys: []KeyRecord{
			{Type: "common/buildings", ID: "building_x", Winner: "mod_b", Manual: true,
				Sources: map[string]string{"mod_a": Hash(1), "mod_b": Hash(0xdeadbeefcafef00d)}},
			{Type: "localisation/english", ID: "KEY", Winner: "mod_a", Skipped: true,
				Sources: map[string]string{"mod_a": Hash(2)}},
		},
	}
}

func TestWriteThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := sample()
	if err := Write(dir, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, ok := Load(dir)
	if !ok {
		t.Fatal("Load reported no usable manifest right after Write")
	}
	if got.FormatVersion != FormatVersion || got.HashVersion != HashVersion {
		t.Errorf("versions = %d/%d, want %d/%d", got.FormatVersion, got.HashVersion, FormatVersion, HashVersion)
	}
	if got.Generation != 3 || got.GeneratedAt != 1_700_000_000 || got.GameVersion != "v4.4.6" {
		t.Errorf("header fields not preserved: %+v", got)
	}
	if got.Mods["mod_a"].Version != "1.2" || got.Mods["mod_b"].Name != "Mod B" {
		t.Errorf("mods not preserved: %+v", got.Mods)
	}
	if len(got.Keys) != 2 {
		t.Fatalf("keys = %d, want 2", len(got.Keys))
	}
	k := got.Keys[0]
	if k.Type != "common/buildings" || k.ID != "building_x" || k.Winner != "mod_b" || !k.Manual || k.Skipped {
		t.Errorf("first key wrong: %+v", k)
	}
	if k.Sources["mod_b"] != "deadbeefcafef00d" {
		t.Errorf("a 64-bit hash must survive as exact hex, got %q", k.Sources["mod_b"])
	}
	if !got.Keys[1].Skipped {
		t.Error("Skipped flag lost")
	}
}

func TestWrittenFileIsCommentedAndOneKeyPerLine(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, sample()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "// Parallax Mod Manager - patch manifest.") {
		t.Errorf("expected an explanatory comment header, got %q", text[:40])
	}
	for _, field := range []string{"formatVersion", "hashVersion", "generation", "generatedAt", "gameVersion", "mods", "keys", "winner", "manual", "skipped", "sources"} {
		if !strings.Contains(text, "// "+field) && !strings.Contains(text, "//   "+field) && !strings.Contains(text, "//   type, id") {
			t.Errorf("header doesn't document %q", field)
		}
	}
	if n := strings.Count(text, `"building_x"`); n != 1 {
		t.Errorf("a key should be on exactly one line, found its id %d times", n)
	}
}

func TestLoadMissingCorruptOrNewerFormatIsNotUsable(t *testing.T) {
	if _, ok := Load(t.TempDir()); ok {
		t.Error("an empty dir must not report a manifest")
	}

	corrupt := t.TempDir()
	if err := os.WriteFile(filepath.Join(corrupt, FileName), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(corrupt); ok {
		t.Error("a corrupt manifest must not be reported as usable")
	}

	newer := t.TempDir()
	if err := os.WriteFile(filepath.Join(newer, FileName), []byte(`{"formatVersion": 99, "keys": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(newer); ok {
		t.Error("a manifest from a newer format must not be read")
	}
}

func TestHashIsFixedWidthHex(t *testing.T) {
	if got := Hash(255); got != "00000000000000ff" {
		t.Errorf("Hash(255) = %q", got)
	}
}
