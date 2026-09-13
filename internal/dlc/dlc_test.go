package dlc

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDLC(t *testing.T, installDir, folder, filename, content string) {
	t.Helper()
	dir := filepath.Join(installDir, "dlc", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// realHorizonSignalDLC is byte-for-byte the real .dlc file confirmed on a
// real Stellaris install (steamapps/common/Stellaris/dlc/dlc013_horizon_signal).
const realHorizonSignalDLC = `name = "Horizon Signal"
localizable_name = "DLC_HORIZON_SIGNAL"
archive = "dlc/dlc013_horizon_signal/dlc013.zip"
steam_id = 554350
msgr_id = ""
rail_id = 2000069
pops_id = "horizon_signal"
affects_checksum = no
affects_compatability = yes
zip_checksum = "d6a7caa23337817776b8750b5167a784"
third_party_content = no
category="content_pack"
thumbnail = "thumbnail.png"`

func TestDiscoverParsesRealDLCFile(t *testing.T) {
	installDir := t.TempDir()
	writeDLC(t, installDir, "dlc013_horizon_signal", "dlc013_horizon_signal.dlc", realHorizonSignalDLC)

	entries, err := Discover(installDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.ID != "dlc013_horizon_signal" {
		t.Errorf("ID = %q, want the folder name", e.ID)
	}
	if e.Name != "Horizon Signal" {
		t.Errorf("Name = %q, want %q", e.Name, "Horizon Signal")
	}
	if !e.Installed {
		t.Error("Installed = false, want true - Discover only ever finds real, locally-installed DLC")
	}
	if e.SteamID != "554350" {
		t.Errorf("SteamID = %q, want %q", e.SteamID, "554350")
	}
	if e.Category != "content_pack" {
		t.Errorf("Category = %q, want %q", e.Category, "content_pack")
	}
	if e.SizeBytes != int64(len(realHorizonSignalDLC)) {
		t.Errorf("SizeBytes = %d, want %d (just the .dlc file itself here)", e.SizeBytes, len(realHorizonSignalDLC))
	}
}

// TestDiscoverSizeBytesReflectsRealContentNotJustMetadata pins a real
// finding: two DLC folders on a real Stellaris install can differ by
// orders of magnitude (a free bonus pack's folder is tens of KB - just
// metadata and a thumbnail - while a full paid expansion's real archive
// is tens of megabytes) - SizeBytes must sum every real file under the
// folder, not just the .dlc metadata file itself.
func TestDiscoverSizeBytesReflectsRealContentNotJustMetadata(t *testing.T) {
	installDir := t.TempDir()
	writeDLC(t, installDir, "dlc014_utopia", "dlc014_utopia.dlc", `name = "Utopia"`)
	if err := os.WriteFile(filepath.Join(installDir, "dlc", "dlc014_utopia", "dlc014.zip"), make([]byte, 5000), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err := Discover(installDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	want := int64(len(`name = "Utopia"`) + 5000)
	if entries[0].SizeBytes != want {
		t.Errorf("SizeBytes = %d, want %d (the .dlc file plus the real archive)", entries[0].SizeBytes, want)
	}
}

func TestDiscoverFindsMultipleDLC(t *testing.T) {
	installDir := t.TempDir()
	writeDLC(t, installDir, "dlc013_horizon_signal", "dlc013_horizon_signal.dlc", realHorizonSignalDLC)
	writeDLC(t, installDir, "dlc015_anniversary", "dlc015_anniversary.dlc", `name = "Anniversary Portraits"
category = "content_pack"`)

	entries, err := Discover(installDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
}

func TestDiscoverMissingDlcFolderIsNotAnError(t *testing.T) {
	entries, err := Discover(t.TempDir())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if entries == nil {
		t.Error("entries is nil, want a real empty slice (marshals as JSON null, not [])")
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %+v", entries)
	}
}

func TestDiscoverSkipsFolderWithNoDLCFile(t *testing.T) {
	installDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(installDir, "dlc", "not_a_dlc"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeDLC(t, installDir, "real_one", "real_one.dlc", `name = "Real One"`)

	entries, err := Discover(installDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "Real One" {
		t.Errorf("entries = %+v, want exactly [Real One]", entries)
	}
}

func TestDiscoverSkipsMalformedDLCFile(t *testing.T) {
	installDir := t.TempDir()
	writeDLC(t, installDir, "broken", "broken.dlc", `name = "unterminated string`)
	writeDLC(t, installDir, "good", "good.dlc", `name = "Good DLC"`)

	entries, err := Discover(installDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "Good DLC" {
		t.Errorf("entries = %+v, want exactly [Good DLC]", entries)
	}
}
