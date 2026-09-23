// Package loverslabinstall extracts a downloaded LoversLab archive into a mod's
// content folder, and works out the classic-format descriptor an install needs - the
// archive's own, if it shipped one, or a synthesized one otherwise. See
// loverslabinstall.go (main package, one layer up) for the download itself and the
// rest of the install flow these pieces plug into: resolving where to install,
// writing the game's own stub, and recording the install for later update checks.
package loverslabinstall

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

// ErrNotAZip means the downloaded file isn't a zip archive - this app only knows how
// to install zip archives (see docs/loverslab.md); anything else needs to be
// extracted by hand.
var ErrNotAZip = errors.New("this download isn't a .zip archive - this app can only install zip files right now; open the download and extract it yourself")

// ExtractResult reports what ExtractZip actually did.
type ExtractResult struct {
	// Files is how many files were written into destDir.
	Files int
	// StubDescriptor is a sibling "<name>.mod" file's own content, when the archive
	// packaged its content as a ready-to-drop-in pair - one top-level folder plus one
	// top-level .mod file beside it, exactly how a Paradox mod folder is normally
	// laid out (mod/<name>.mod next to mod/<name>/) - confirmed as a real, common
	// packaging style against a real download (see docs/loverslab.md), not a
	// contrived edge case. That file is deliberately not extracted into destDir along
	// with everything else: it belongs beside the mod's content folder as its own
	// stub (see ResolveDescriptor), not inside it, and even if it were extracted
	// alongside the content, the game would never read a stray .mod file sitting
	// inside another mod's own content folder. Empty when the archive had no such
	// sibling file.
	StubDescriptor []byte
}

// ExtractZip extracts archivePath into destDir (created if it doesn't already exist).
// If every entry in the archive shares one common top-level folder, that wrapping
// folder is stripped so destDir ends up holding the mod's content directly, not
// content nested one level too deep for the game to find - the same way most
// "install from zip" tools behave. See ExtractResult.StubDescriptor for the one
// related, real-world packaging style this also recognizes.
func ExtractZip(archivePath, destDir string) (ExtractResult, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return ExtractResult{}, fmt.Errorf("%w (%v)", ErrNotAZip, err)
	}
	defer r.Close()

	prefix, stub := contentLayout(r.File)

	var result ExtractResult
	if stub != nil {
		rc, err := stub.Open()
		if err != nil {
			return ExtractResult{}, fmt.Errorf("reading %q from the archive: %w", stub.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return ExtractResult{}, fmt.Errorf("reading %q from the archive: %w", stub.Name, err)
		}
		result.StubDescriptor = data
	}

	for _, f := range r.File {
		if f == stub {
			continue // its own content is ExtractResult.StubDescriptor, not a file in destDir
		}
		name := strings.TrimPrefix(f.Name, prefix)
		if name == "" {
			continue // the wrapping folder's own entry, already accounted for by destDir itself
		}
		target, err := safeJoin(destDir, name)
		if err != nil {
			return result, err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return result, fmt.Errorf("creating %q: %w", target, err)
			}
			continue
		}
		if err := extractFile(f, target); err != nil {
			return result, err
		}
		result.Files++
	}
	return result, nil
}

// contentLayout looks at every top-level entry in files (nothing before the first
// "/" in its name) and returns how to extract it:
//
//   - One top-level folder, nothing else: prefix strips that wrapper (the plain
//     "MyMod/everything-inside" case).
//   - One top-level folder plus exactly one top-level file named after that same
//     folder (e.g. "mymod/" alongside "mymod.mod") with a ".mod" extension: prefix
//     strips the folder the same way, and stub is that file - the "ready to drop
//     into your own mod folder" pairing (see ExtractResult.StubDescriptor). The name
//     must actually match the folder's own name, not just end in ".mod" - a flat-root
//     archive can perfectly normally have both its own "descriptor.mod" at the root
//     and some unrelated subfolder (e.g. "common/"), and that combination must not be
//     mistaken for this pattern.
//   - Anything else (content already at the root, multiple top-level folders, a
//     stray file that doesn't name-match the one folder): prefix is "", stub is nil -
//     extract exactly as laid out in the archive.
func contentLayout(files []*zip.File) (prefix string, stub *zip.File) {
	var folder string
	var strayFiles []*zip.File
	for _, f := range files {
		name := strings.TrimPrefix(f.Name, "/")
		i := strings.Index(name, "/")
		if i < 0 {
			strayFiles = append(strayFiles, f)
			continue
		}
		first := name[:i+1]
		if folder == "" {
			folder = first
		} else if folder != first {
			return "", nil // more than one top-level folder - nothing to strip
		}
	}
	if folder == "" {
		return "", nil // content already at the root
	}
	switch len(strayFiles) {
	case 0:
		return folder, nil // a plain wrapping folder, nothing else at the top level
	case 1:
		folderName := strings.TrimSuffix(folder, "/")
		strayName := strings.TrimSuffix(strayFiles[0].Name, filepath.Ext(strayFiles[0].Name))
		if strings.EqualFold(filepath.Ext(strayFiles[0].Name), ".mod") && strings.EqualFold(strayName, folderName) {
			return folder, strayFiles[0]
		}
	}
	// One folder plus a stray file that doesn't name-match it (e.g. this archive's
	// own top-level "descriptor.mod" alongside an unrelated "common/" folder) is
	// ordinary flat-root content, not a wrapper to strip - stripping "common/" here
	// would wrongly relocate its own files up a level.
	return "", nil
}

// safeJoin joins dir and name, refusing a name that would extract outside dir (a
// malicious or malformed archive entry using ".." or an absolute path - the "zip
// slip" vulnerability).
func safeJoin(dir, name string) (string, error) {
	cleanDir := filepath.Clean(dir)
	target := filepath.Join(cleanDir, name)
	if target != cleanDir && !strings.HasPrefix(target, cleanDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("this archive's entry %q would extract outside the mod's own folder - refusing to install it", name)
	}
	return target, nil
}

func extractFile(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating %q: %w", filepath.Dir(target), err)
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("reading %q from the archive: %w", f.Name, err)
	}
	defer rc.Close()

	perm := f.Mode().Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("writing %q: %w", target, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("writing %q: %w", target, err)
	}
	return nil
}

// ResolveDescriptor works out the classic-format descriptor an installed mod's stub
// (and, when synthesized, its own descriptor.mod) should carry, preferring - in order
// - stubDescriptor (ExtractZip's own result: a sibling "<name>.mod" file the archive
// packaged beside its content folder, the strongest signal of the author's own
// intended descriptor), then contentDir/descriptor.mod (a mod whose content folder
// describes itself), then a fresh one built from title/remoteFileID when neither
// exists. Every field is kept exactly as the author wrote it, except RemoteFileID and
// Path, which are always this app's own (a third-party archive has no way to know
// either). hadOwnDescriptor reports whether either real source was found: the caller
// only needs to separately write contentDir/descriptor.mod itself when it's false -
// one already on disk (or recovered from the sibling stub) is never duplicated.
func ResolveDescriptor(contentDir string, stubDescriptor []byte, title, remoteFileID string) (desc mod.Descriptor, hadOwnDescriptor bool, err error) {
	if len(stubDescriptor) > 0 {
		parsed, err := mod.ParseDescriptor(stubDescriptor, mod.DescriptorClassic)
		if err != nil {
			return mod.Descriptor{}, false, fmt.Errorf("the archive's own stub descriptor could not be read: %w", err)
		}
		return finishDescriptor(parsed, contentDir, title, remoteFileID), true, nil
	}

	descPath := filepath.Join(contentDir, "descriptor.mod")
	data, readErr := os.ReadFile(descPath)
	if readErr == nil {
		parsed, err := mod.ParseDescriptor(data, mod.DescriptorClassic)
		if err != nil {
			return mod.Descriptor{}, false, fmt.Errorf("the archive's own descriptor.mod could not be read: %w", err)
		}
		return finishDescriptor(parsed, contentDir, title, remoteFileID), true, nil
	}
	if !os.IsNotExist(readErr) {
		return mod.Descriptor{}, false, fmt.Errorf("reading %q: %w", descPath, readErr)
	}
	return mod.Descriptor{Name: title, Path: contentDir, RemoteFileID: remoteFileID}, false, nil
}

func finishDescriptor(parsed mod.Descriptor, contentDir, title, remoteFileID string) mod.Descriptor {
	parsed.RemoteFileID = remoteFileID
	parsed.Path = contentDir
	if strings.TrimSpace(parsed.Name) == "" {
		parsed.Name = title
	}
	return parsed
}
