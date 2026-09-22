package modedit

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// FolderName turns a mod's display name into a safe, single path component: the same character
// class internal/playset and internal/collection already replace (/ \ : * ? " < > |), trimmed,
// falling back to "_" when that leaves nothing (their own rule) - plus rejecting "." and ".."
// outright, which that replacement alone does not catch and which matters here since the result
// becomes a real new directory next to the game's own files.
func FolderName(name string) (string, error) {
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	safe := strings.TrimSpace(b.String())
	if safe == "" {
		safe = "_"
	}
	if safe == "." || safe == ".." {
		return "", fmt.Errorf("modedit: %q is not a usable folder name", name)
	}
	return safe, nil
}

// NewFiles is what creating a mod writes from scratch: the content descriptor (via the same
// newDescriptor a missing descriptor.mod is already created with - never a path=, since a
// descriptor inside the mod's own folder never names its own folder) and, only when stubPath is
// not "", its stub (see NewStub). Both are verified by parsing them back before being returned,
// same as Plan.
func NewFiles(want Fields, contentDir, stubPath string) ([]FileEdit, error) {
	want = want.Normalized()
	if err := want.Validate(); err != nil {
		return nil, err
	}

	descPath := filepath.Join(contentDir, "descriptor.mod")
	descText := newDescriptor(want, "")
	if err := verify(descText, want, ""); err != nil {
		return nil, fmt.Errorf("modedit: the new descriptor would not read back as asked (%v); nothing was created", err)
	}
	edits := []FileEdit{{Path: descPath, Kind: KindDescriptor, Create: true, After: descText}}

	if stubPath != "" {
		stub, err := NewStub(want, contentDir, stubPath)
		if err != nil {
			return nil, err
		}
		edits = append(edits, stub)
	}
	return edits, nil
}

// NewStub builds a fresh stub file edit for a mod living at contentDir: the file the game itself
// reads, in its own mod folder, with path= set to contentDir (the one field a stub always needs
// that a mod's own descriptor.mod never does) - mirrors internal/library/patch.go's GeneratePatch
// (stub := desc; stub.Path = patchContentDir), just for one mod instead of the combined patch.
// Used both by NewFiles (a brand-new mod placed in the game's own mod folder) and by anything
// else that needs a stub with no existing one to edit in place - a duplicated mod's stub is
// never copied from its source, since the source's own stub (if it has one) lives outside the
// folder being copied, so it is always created fresh here too. Verified by parsing it back
// before being returned, same as Plan.
func NewStub(want Fields, contentDir, stubPath string) (FileEdit, error) {
	want = want.Normalized()
	if err := want.Validate(); err != nil {
		return FileEdit{}, err
	}
	text := string(mod.WriteClassicDescriptor(mod.Descriptor{
		Name:             want.Name,
		Path:             contentDir,
		Version:          want.Version,
		SupportedVersion: want.SupportedVersion,
		Tags:             want.Tags,
		Dependencies:     want.Dependencies,
		ReplacePath:      want.ReplacePaths,
	}))
	if err := verify(text, want, ""); err != nil {
		return FileEdit{}, fmt.Errorf("modedit: the new stub would not read back as asked (%v); nothing was created", err)
	}
	return FileEdit{Path: stubPath, Kind: KindStub, Create: true, After: text}, nil
}
