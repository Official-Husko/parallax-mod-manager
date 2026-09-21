package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/atomicfile"
	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// The game finds a classic-format mod through a small "stub" file in its own mod folder
// (mod/<id>.mod) whose path= line says where the mod's content is; dlc_load.json only names
// the stub. Two things go wrong with mods that live outside that folder, and both leave a mod
// looking fine in the manager but never loading in the game:
//
//   - the stub's path is stale: the mod library moved (another drive, another mount name, a
//     renamed folder) and the stub still points at the old place. The manager reconnects the
//     mod to its real folder (see findContentByName) but the game only reads the stub.
//   - there is no stub at all: a mod found in an extra mod folder has a descriptor.mod inside
//     its own folder and nothing in the game's mod folder points at it.
//
// EnsureStub fixes both, for the mods a launch is about to enable.

// StubChange is what EnsureStub did to one mod's stub.
type StubChange struct {
	ModID string
	Name  string
	// File is the stub's full path.
	File string
	// OldPath is the path the stub declared before ("" when it declared none, or was created).
	OldPath string
	// NewPath is the content folder it points at now.
	NewPath string
	// Created is true when the stub did not exist and was written.
	Created bool
}

// EnsureStub makes sure the game's mod folder (modDir) has a stub through which the game can
// load m, and reports what it changed (changed is false when nothing needed doing):
//
//   - m has no stub and is defined by a descriptor.mod in its own folder (an extra mod folder,
//     an unlinked Workshop item): one is written, pointing at that folder.
//   - m's stub declares a path that does not exist on disk while m's content was found
//     elsewhere: only its path= line is rewritten, everything else in the file (name, tags,
//     version, comments) stays as it was. A stub with no path= line gets one.
//   - a stub whose path exists is never touched, whatever else is wrong with it.
//
// Nothing is done for a mod whose content was not found (ContentMissing): there is nothing to
// point at. The rewrite is atomic and keeps the file's permissions.
func EnsureStub(m mod.Mod, modDir string) (change StubChange, changed bool, err error) {
	if m.ContentMissing || m.ContentPath == "" {
		return StubChange{}, false, nil
	}
	name := m.Descriptor.Name
	if name == "" {
		name = m.ID
	}
	stub := StubChange{ModID: m.ID, Name: name, NewPath: m.ContentPath}

	inModDir := filepath.Clean(filepath.Dir(m.DescriptorPath)) == filepath.Clean(modDir)
	if inModDir {
		stub.File = m.DescriptorPath
	} else {
		stub.File = filepath.Join(modDir, m.ID+".mod")
	}

	data, readErr := os.ReadFile(stub.File)
	if os.IsNotExist(readErr) {
		if inModDir {
			return StubChange{}, false, nil // the stub the mod was read from is gone: nothing to mend
		}
		desc := m.Descriptor
		desc.Path = m.ContentPath
		if desc.Name == "" {
			desc.Name = filepath.Base(m.ContentPath)
		}
		if _, err := atomicfile.Write(modDir, filepath.Base(stub.File), mod.WriteClassicDescriptor(desc)); err != nil {
			return StubChange{}, false, fmt.Errorf("scan: writing the stub for %q: %w", name, err)
		}
		stub.Created = true
		return stub, true, nil
	}
	if readErr != nil {
		return StubChange{}, false, fmt.Errorf("scan: reading the stub of %q: %w", name, readErr)
	}

	declared := declaredStubPath(data, modDir)
	stub.OldPath = declared
	if declared != "" {
		if _, err := os.Stat(declared); err == nil {
			return StubChange{}, false, nil // it works
		}
	}
	if strings.ContainsAny(m.ContentPath, "\r\n") {
		return StubChange{}, false, fmt.Errorf("scan: %q is not a usable folder path", m.ContentPath)
	}

	fixed := setStubPath(data, m.ContentPath)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(stub.File); err == nil {
		mode = info.Mode().Perm()
	}
	written, err := atomicfile.Write(filepath.Dir(stub.File), filepath.Base(stub.File), fixed)
	if err != nil {
		return StubChange{}, false, fmt.Errorf("scan: repairing the stub of %q: %w", name, err)
	}
	_ = os.Chmod(written, mode)
	return stub, true, nil
}

// declaredStubPath is the path a stub's text declares, resolved as the game would
// (a relative one against modDir); "" when it declares none.
func declaredStubPath(data []byte, modDir string) string {
	desc, err := mod.ParseDescriptor(data, mod.DescriptorClassic)
	if err != nil || desc.Path == "" {
		return ""
	}
	if filepath.IsAbs(desc.Path) {
		return desc.Path
	}
	return filepath.Join(modDir, desc.Path)
}

// pathLine matches a stub's path= line (not archive=, which names a file, not a folder).
var pathLine = regexp.MustCompile(`(?m)^([ \t]*path[ \t]*=[ \t]*)"(?:[^"\\\n]|\\.)*"`)

// setStubPath returns data with its path= line set to newPath, or with one added at the end when
// there is none. Everything else is kept byte for byte.
func setStubPath(data []byte, newPath string) []byte {
	quoted := mod.QuoteClausewitz(newPath)
	if loc := pathLine.FindSubmatchIndex(data); loc != nil {
		out := make([]byte, 0, len(data)+len(quoted))
		out = append(out, data[:loc[3]]...)
		out = append(out, quoted...)
		return append(out, data[loc[1]:]...)
	}
	out := append([]byte(nil), data...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(out, "path="+quoted+"\n"...)
}
