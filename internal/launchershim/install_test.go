package launchershim

import (
	"os"
	"path/filepath"
	"testing"
)

// A stand-in for a real dowser - anything that isn't our shim marker.
const fakeDowserContent = "this is a totally real Paradox dowser binary, honest"

// A stand-in for a real, built launcher-shim binary - just needs to contain
// shimMarker for looksLikeOurShim to recognize it.
const fakeShimContent = "fake shim binary containing " + shimMarker + " like a real build would"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func TestDetectNotInstalled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "dowser"), fakeDowserContent)

	state, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if state != StateNotInstalled {
		t.Errorf("state = %q, want %q", state, StateNotInstalled)
	}
}

func TestDetectNoEntryPointAtAll(t *testing.T) {
	if _, err := Detect(t.TempDir()); err == nil {
		t.Fatal("expected an error when neither dowser nor dowser.exe exists")
	}
}

func TestInstallFromFreshInstall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "dowser"), fakeDowserContent)

	shimSrc := filepath.Join(t.TempDir(), "shim-source")
	writeFile(t, shimSrc, fakeShimContent)

	if err := installFrom(dir, shimSrc); err != nil {
		t.Fatalf("installFrom: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(dir, "dowser"))
	if err != nil {
		t.Fatalf("reading installed dowser: %v", err)
	}
	if string(installed) != fakeShimContent {
		t.Errorf("installed dowser content = %q, want the shim's own content", installed)
	}

	backup, err := os.ReadFile(filepath.Join(dir, "dowser.original"))
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backup) != fakeDowserContent {
		t.Errorf("backup content = %q, want the original dowser's content", backup)
	}

	if _, err := os.Stat(filepath.Join(dir, noticeFileName)); err != nil {
		t.Errorf("notice file was not written: %v", err)
	}

	state, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect after install: %v", err)
	}
	if state != StateInstalled {
		t.Errorf("state after install = %q, want %q", state, StateInstalled)
	}
}

func TestInstallFromIsANoOpWhenAlreadyInstalled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "dowser"), fakeShimContent)
	writeFile(t, filepath.Join(dir, "dowser.original"), fakeDowserContent)

	shimSrc := filepath.Join(t.TempDir(), "shim-source")
	writeFile(t, shimSrc, fakeShimContent)

	if err := installFrom(dir, shimSrc); err != nil {
		t.Fatalf("installFrom: %v", err)
	}
	// Nothing should have changed - still the same backup, still the same shim.
	backup, err := os.ReadFile(filepath.Join(dir, "dowser.original"))
	if err != nil || string(backup) != fakeDowserContent {
		t.Errorf("backup was disturbed by a no-op install: %q, %v", backup, err)
	}
}

func TestInstallFromRepairsAStaleInstall(t *testing.T) {
	dir := t.TempDir()
	// Steam's own "Verify integrity of game files" restored the real dowser, but
	// our backup is still sitting there from before.
	writeFile(t, filepath.Join(dir, "dowser"), fakeDowserContent)
	writeFile(t, filepath.Join(dir, "dowser.original"), fakeDowserContent)

	state, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if state != StateNeedsRepair {
		t.Fatalf("state = %q, want %q", state, StateNeedsRepair)
	}

	shimSrc := filepath.Join(t.TempDir(), "shim-source")
	writeFile(t, shimSrc, fakeShimContent)
	if err := installFrom(dir, shimSrc); err != nil {
		t.Fatalf("installFrom (repair): %v", err)
	}

	state, err = Detect(dir)
	if err != nil {
		t.Fatalf("Detect after repair: %v", err)
	}
	if state != StateInstalled {
		t.Errorf("state after repair = %q, want %q", state, StateInstalled)
	}
	// The original backup must survive a repair untouched.
	backup, err := os.ReadFile(filepath.Join(dir, "dowser.original"))
	if err != nil || string(backup) != fakeDowserContent {
		t.Errorf("backup was disturbed by a repair: %q, %v", backup, err)
	}
}

func TestInstallFromRefusesAnUnrecognizedExistingBackup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "dowser"), fakeDowserContent)
	// A backup that is itself our shim - not a real backup at all, something is
	// wrong here that installFrom must never paper over automatically.
	writeFile(t, filepath.Join(dir, "dowser.original"), fakeShimContent)

	shimSrc := filepath.Join(t.TempDir(), "shim-source")
	writeFile(t, shimSrc, fakeShimContent)

	state, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if state != StateUnknown {
		t.Fatalf("state = %q, want %q", state, StateUnknown)
	}
	if err := installFrom(dir, shimSrc); err == nil {
		t.Fatal("expected installFrom to refuse an unrecognized existing backup")
	}
	// Nothing should have been touched.
	dowser, _ := os.ReadFile(filepath.Join(dir, "dowser"))
	if string(dowser) != fakeDowserContent {
		t.Errorf("the real dowser was touched despite refusing to install: %q", dowser)
	}
}

func TestRemoveRestoresTheBackupAndDeletesTheNotice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "dowser"), fakeShimContent)
	writeFile(t, filepath.Join(dir, "dowser.original"), fakeDowserContent)
	writeFile(t, filepath.Join(dir, noticeFileName), "notice")

	if err := Remove(dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	dowser, err := os.ReadFile(filepath.Join(dir, "dowser"))
	if err != nil || string(dowser) != fakeDowserContent {
		t.Errorf("dowser after Remove = %q, %v, want the original content back", dowser, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dowser.original")); !os.IsNotExist(err) {
		t.Errorf("backup file was not consumed by Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, noticeFileName)); !os.IsNotExist(err) {
		t.Errorf("notice file was not removed: %v", err)
	}
}

func TestRemoveRefusesWithNoBackup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "dowser"), fakeDowserContent)
	if err := Remove(dir); err == nil {
		t.Fatal("expected Remove to refuse when there is no backup to restore")
	}
}

func TestShimBinaryNamePicksThePlatformMatchingTheEntryPoint(t *testing.T) {
	if got := shimBinaryName("dowser"); got != "launcher-shim-linux-amd64" {
		t.Errorf("shimBinaryName(dowser) = %q", got)
	}
	if got := shimBinaryName("dowser.exe"); got != "launcher-shim-windows-amd64.exe" {
		t.Errorf("shimBinaryName(dowser.exe) = %q", got)
	}
}
