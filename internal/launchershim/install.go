package launchershim

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// entryPointNames are the two real names a game's launcher entry point can have.
// Detect/Install pick whichever one actually exists in a game's install directory,
// not the host OS this app itself happens to be running on - a Linux build of this
// app can still be managing a Windows/Proton game's install, which needs
// "dowser.exe", not "dowser".
var entryPointNames = []string{"dowser", "dowser.exe"}

// backupSuffix names the file Install backs up a game's real dowser/dowser.exe to,
// right next to it.
const backupSuffix = ".original"

// noticeFileName is the small, plain-text note Install leaves in the game's own
// install folder next to the shim - never a copy of this project's source, only an
// explanation and a link (see noticeText).
const noticeFileName = "PARALLAX-MOD-MANAGER-README.txt"

// shimMarker is a string every real build of companions/launcher-shim contains
// (its own repoURL constant, already present for --source's own output) - used
// here as a cheap, reliable "is this file actually our shim" check, rather than
// tracking a checksum that would need to change every time the shim itself is
// rebuilt. The real Paradox dowser/dowser.exe will never contain this string.
const shimMarker = "https://github.com/Official-Husko/parallax-mod-manager"

// State describes what Detect found sitting in a game's own launcher-entry-point
// slot.
type State string

const (
	// StateNotInstalled is the real dowser/dowser.exe, untouched - no backup exists.
	StateNotInstalled State = "not-installed"
	// StateInstalled is our shim, with a real, legitimate-looking backup beside it.
	StateInstalled State = "installed"
	// StateNeedsRepair is a real backup existing next to something that is *not*
	// our shim right now - most commonly Steam's own "Verify integrity of game
	// files" quietly restoring the original dowser/dowser.exe. One click (Fix)
	// away from StateInstalled again.
	StateNeedsRepair State = "needs-repair"
	// StateUnknown is anything else - a backup file that doesn't look like a real,
	// legitimate prior backup, or no dowser/dowser.exe at all. Nothing here is
	// ever touched automatically in this state.
	StateUnknown State = "unknown"
)

// entryPoint finds which of entryPointNames actually exists in installDir, and
// works out its backup's path either way (whether or not that backup exists yet).
func entryPoint(installDir string) (name, path, backupPath string, found bool) {
	for _, n := range entryPointNames {
		p := filepath.Join(installDir, n)
		if _, err := os.Stat(p); err == nil {
			return n, p, p + backupSuffix, true
		}
	}
	return "", "", "", false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// looksLikeOurShim reports whether the file at path is a real build of
// companions/launcher-shim, by checking for shimMarker's presence in its bytes.
func looksLikeOurShim(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return bytes.Contains(data, []byte(shimMarker)), nil
}

// looksLikeARealBackup reports whether path is plausibly a real backed-up
// dowser/dowser.exe: it exists, is not empty, and is not itself our shim (a
// backup should never be a copy of the shim - if it is, something other than this
// package's own Install put it there).
func looksLikeARealBackup(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, nil //nolint:nilerr // missing is just "no backup", not an error
	}
	if info.Size() == 0 {
		return false, nil
	}
	isShim, err := looksLikeOurShim(path)
	if err != nil {
		return false, err
	}
	return !isShim, nil
}

// Detect reports installDir's current State - see the State constants for what
// each one means and what (if anything) it is safe to do next.
func Detect(installDir string) (State, error) {
	_, path, backupPath, found := entryPoint(installDir)
	if !found {
		return StateUnknown, fmt.Errorf("launchershim: no dowser/dowser.exe found in %s", installDir)
	}

	// "No backup file at all" and "a backup file exists but doesn't look
	// legitimate" must stay distinct here - the first is a genuinely fresh,
	// untouched install; the second is something worth refusing to touch
	// automatically, even though both make looksLikeARealBackup return false.
	backupExists := fileExists(backupPath)
	backupLegit := false
	if backupExists {
		var err error
		backupLegit, err = looksLikeARealBackup(backupPath)
		if err != nil {
			return StateUnknown, err
		}
	}
	isShim, err := looksLikeOurShim(path)
	if err != nil {
		return StateUnknown, err
	}

	switch {
	case !backupExists && !isShim:
		return StateNotInstalled, nil
	case backupExists && backupLegit && isShim:
		return StateInstalled, nil
	case backupExists && backupLegit && !isShim:
		return StateNeedsRepair, nil
	default: // an illegitimate-looking backup, or our shim with no backup at all
		return StateUnknown, nil
	}
}

// shimBinaryName maps a game's entry point filename to the pre-built companion
// binary that replaces it - see the root build.sh, which places both platform
// builds side by side in a "companions" folder next to this app's own executable,
// regardless of which OS this app itself happens to be running on.
func shimBinaryName(entryPointName string) string {
	if entryPointName == "dowser.exe" {
		return "launcher-shim-windows-amd64.exe"
	}
	return "launcher-shim-linux-amd64"
}

// shimSourcePath resolves where the pre-built shim binary for entryPointName lives,
// relative to this app's own executable.
func shimSourcePath(entryPointName string) (string, error) {
	selfPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("launchershim: finding this app's own path: %w", err)
	}
	return filepath.Join(filepath.Dir(selfPath), "companions", shimBinaryName(entryPointName)), nil
}

// Install installs the shim in place of installDir's own dowser/dowser.exe -
// backing up the real one first if this is a fresh install, or repairing an
// install Detect found in StateNeedsRepair. A no-op returning nil if it's already
// StateInstalled. Refuses StateUnknown outright, rather than guessing.
func Install(installDir string) error {
	name, _, _, found := entryPoint(installDir)
	if !found {
		return fmt.Errorf("launchershim: no dowser/dowser.exe found in %s", installDir)
	}
	src, err := shimSourcePath(name)
	if err != nil {
		return err
	}
	return installFrom(installDir, src)
}

// installFrom is Install's own real work, given an explicit shim source path -
// pulled out so tests never depend on this app's own real installed location
// (which shimSourcePath resolves via os.Executable, not present in a test binary).
func installFrom(installDir, shimSourcePath string) error {
	name, path, backupPath, found := entryPoint(installDir)
	if !found {
		return fmt.Errorf("launchershim: no dowser/dowser.exe found in %s", installDir)
	}

	state, err := Detect(installDir)
	if err != nil {
		return err
	}
	switch state {
	case StateInstalled:
		return nil
	case StateUnknown:
		return fmt.Errorf("launchershim: %s doesn't look safe to touch automatically - if you're sure, remove any %s file by hand first", path, filepath.Base(backupPath))
	case StateNeedsRepair:
		return repairInPlace(name, path, shimSourcePath)
	}

	shimData, err := os.ReadFile(shimSourcePath)
	if err != nil {
		return fmt.Errorf("launchershim: reading the shim binary to install: %w", err)
	}

	// Back up the real file first (atomic rename, same filesystem) - only once we
	// know the shim binary itself is actually readable.
	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("launchershim: backing up %s: %w", path, err)
	}

	if err := os.WriteFile(path, shimData, 0o755); err != nil {
		_ = os.Rename(backupPath, path) // put the real one back rather than leaving the game unlaunchable
		return fmt.Errorf("launchershim: writing the shim in place of %s: %w", path, err)
	}

	if ok, err := looksLikeOurShim(path); err != nil || !ok {
		_ = os.Remove(path)
		_ = os.Rename(backupPath, path)
		return fmt.Errorf("launchershim: the installed shim did not verify correctly - restored the original %s", name)
	}

	if err := writeNotice(installDir, name, filepath.Base(backupPath)); err != nil {
		// The swap itself succeeded; a failed notice is worth knowing about but
		// never a reason to undo a good install.
		return fmt.Errorf("launchershim: installed, but could not write %s: %w", noticeFileName, err)
	}
	return nil
}

// repairInPlace re-installs the shim over a StateNeedsRepair install (something
// that isn't our shim sitting where our shim should be, with a real backup already
// safely beside it) - the backup itself is never touched.
func repairInPlace(name, path, shimSourcePath string) error {
	shimData, err := os.ReadFile(shimSourcePath)
	if err != nil {
		return fmt.Errorf("launchershim: reading the shim binary to repair with: %w", err)
	}

	// Keep a copy of whatever is currently there in memory (not on disk - the
	// backup file already covers "restore the real thing") only long enough to
	// put it back if writing the shim fails partway through.
	current, readErr := os.ReadFile(path)

	if err := os.WriteFile(path, shimData, 0o755); err != nil {
		if readErr == nil {
			_ = os.WriteFile(path, current, 0o755)
		}
		return fmt.Errorf("launchershim: writing the shim in place of %s: %w", name, err)
	}
	if ok, err := looksLikeOurShim(path); err != nil || !ok {
		if readErr == nil {
			_ = os.WriteFile(path, current, 0o755)
		}
		return fmt.Errorf("launchershim: the repaired shim did not verify correctly")
	}
	return nil
}

// Remove restores installDir's real dowser/dowser.exe from its backup, and
// deletes the notice file Install left behind. Refuses if there's no legitimate
// backup to restore from, rather than deleting the shim and leaving the game
// unlaunchable.
func Remove(installDir string) error {
	name, path, backupPath, found := entryPoint(installDir)
	if !found {
		return fmt.Errorf("launchershim: no dowser/dowser.exe found in %s", installDir)
	}
	ok, err := looksLikeARealBackup(backupPath)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("launchershim: no real backup found for %s - nothing to restore", name)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("launchershim: removing %s: %w", path, err)
	}
	if err := os.Rename(backupPath, path); err != nil {
		return fmt.Errorf("launchershim: restoring %s from its backup: %w", name, err)
	}
	_ = os.Remove(filepath.Join(installDir, noticeFileName)) // best-effort; not restoring it is not a reason to fail
	return nil
}

// noticeText is what Install leaves behind in a game's own install folder - never
// a copy of this project's source, only an explanation and a link.
const noticeText = `This game's own %s was temporarily replaced by Parallax Mod Manager, so Steam
launches the game directly without ever opening the Paradox Launcher. The original
file is saved right next to this one, as "%s" - Parallax Mod Manager restores it
automatically if you remove this from its own Settings > Launch options, or if it
repairs this install after Steam's own "Verify integrity of game files" puts the
original back on its own.

This is never a copy of Parallax Mod Manager's own source code - only a link to it:
https://github.com/Official-Husko/parallax-mod-manager
`

func writeNotice(installDir, entryPointName, backupName string) error {
	return os.WriteFile(filepath.Join(installDir, noticeFileName), []byte(fmt.Sprintf(noticeText, entryPointName, backupName)), 0o644)
}
