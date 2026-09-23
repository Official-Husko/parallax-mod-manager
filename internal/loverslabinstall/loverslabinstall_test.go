package loverslabinstall

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// buildZip writes a zip archive at path containing entries (name -> content; a
// trailing "/" in a name writes a directory entry).
func buildZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range entries {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractZipFlatRoot(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "mod.zip")
	buildZip(t, archivePath, map[string]string{
		"descriptor.mod":            `name="Test Mod"`,
		"common/traits/00_test.txt": "trait content",
	})

	destDir := filepath.Join(dir, "installed")
	res, err := ExtractZip(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if res.Files != 2 {
		t.Errorf("wrote %d files, want 2", res.Files)
	}
	if res.StubDescriptor != nil {
		t.Errorf("StubDescriptor = %q, want none (flat root, no sibling stub)", res.StubDescriptor)
	}
	if data, err := os.ReadFile(filepath.Join(destDir, "descriptor.mod")); err != nil || string(data) != `name="Test Mod"` {
		t.Errorf("descriptor.mod = %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(destDir, "common/traits/00_test.txt")); err != nil || string(data) != "trait content" {
		t.Errorf("common/traits/00_test.txt = %q, %v", data, err)
	}
}

func TestExtractZipStripsAWrappingFolder(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "mod.zip")
	buildZip(t, archivePath, map[string]string{
		"MyMod/":                   "",
		"MyMod/descriptor.mod":     `name="Wrapped Mod"`,
		"MyMod/common/00_test.txt": "content",
	})

	destDir := filepath.Join(dir, "installed")
	res, err := ExtractZip(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if res.StubDescriptor != nil {
		t.Error("expected no sibling stub - just a plain wrapping folder")
	}
	// The wrapping "MyMod/" folder must not appear in the extracted layout - its
	// content lands directly in destDir.
	if _, err := os.Stat(filepath.Join(destDir, "descriptor.mod")); err != nil {
		t.Errorf("descriptor.mod not found directly in destDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "MyMod")); err == nil {
		t.Error("the wrapping folder should have been stripped, not extracted as a subfolder")
	}
}

// A real, common LoversLab packaging style (confirmed against a real download - see
// docs/loverslab.md): the archive holds a ready-to-drop-in pair, a content folder plus
// a sibling "<name>.mod" stub beside it, exactly how a Paradox mod folder is laid out
// for real (mod/<name>.mod next to mod/<name>/). The stub must be recovered as
// ExtractResult.StubDescriptor, not extracted into destDir - a game never reads a
// stray .mod file sitting inside another mod's own content folder, and the content
// folder itself must still be unwrapped so its own files land directly in destDir.
func TestExtractZipRecognizesAContentFolderWithASiblingStub(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "mod.zip")
	buildZip(t, archivePath, map[string]string{
		"mymod/":                "",
		"mymod/common/00_x.txt": "x",
		"mymod.mod":             "name=\"My Mod\"\npath=\"mod/mymod\"\n",
	})

	destDir := filepath.Join(dir, "installed")
	res, err := ExtractZip(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if string(res.StubDescriptor) != "name=\"My Mod\"\npath=\"mod/mymod\"\n" {
		t.Errorf("StubDescriptor = %q, want the sibling .mod file's own content", res.StubDescriptor)
	}
	if res.Files != 1 {
		t.Errorf("Files = %d, want 1 (just common/00_x.txt - the stub itself is not extracted)", res.Files)
	}
	// The content folder is unwrapped, same as a plain wrapping folder would be.
	if _, err := os.Stat(filepath.Join(destDir, "common", "00_x.txt")); err != nil {
		t.Errorf("common/00_x.txt not found directly in destDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "mymod")); err == nil {
		t.Error("the content folder should have been unwrapped, not extracted as a subfolder")
	}
	if _, err := os.Stat(filepath.Join(destDir, "mymod.mod")); err == nil {
		t.Error("the sibling stub should never be extracted into destDir alongside the content")
	}
}

func TestExtractZipDoesNotTreatAnArbitraryTopLevelFileAsAStub(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "mod.zip")
	buildZip(t, archivePath, map[string]string{
		"mymod/":               "",
		"mymod/descriptor.mod": `name="Test"`,
		"README.txt":           "not a stub, just a readme",
	})

	destDir := filepath.Join(dir, "installed")
	res, err := ExtractZip(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if res.StubDescriptor != nil {
		t.Errorf("StubDescriptor = %q, want none (README.txt is not a .mod file)", res.StubDescriptor)
	}
	// Not a recognized layout (a folder plus a non-.mod stray file) - extracted
	// exactly as laid out in the archive, nothing stripped.
	if _, err := os.Stat(filepath.Join(destDir, "mymod", "descriptor.mod")); err != nil {
		t.Errorf("expected the archive's own layout to be kept as-is: %v", err)
	}
}

func TestExtractZipMultipleTopLevelEntriesAreNotTreatedAsAWrapper(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "mod.zip")
	buildZip(t, archivePath, map[string]string{
		"descriptor.mod": `name="Test"`,
		"common/x.txt":   "x",
		"events/y.txt":   "y",
	})

	destDir := filepath.Join(dir, "installed")
	res, err := ExtractZip(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}
	if res.Files != 3 {
		t.Errorf("wrote %d files, want 3", res.Files)
	}
	if _, err := os.Stat(filepath.Join(destDir, "descriptor.mod")); err != nil {
		t.Errorf("descriptor.mod not found: %v", err)
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "evil.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// zip.Writer.Create normalizes ".." itself in some Go versions, so this entry is
	// built with a raw header to guarantee the traversal name actually reaches
	// ExtractZip's own safety check, not silently cleaned up before it.
	fw, err := w.CreateHeader(&zip.FileHeader{Name: "../../etc/evil.txt", Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("evil")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	destDir := filepath.Join(dir, "installed")
	if _, err := ExtractZip(archivePath, destDir); err == nil {
		t.Error("expected a path-traversal entry to be refused")
	}
}

func TestExtractZipRejectsANonZipFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-zip.txt")
	if err := os.WriteFile(path, []byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractZip(path, filepath.Join(dir, "installed")); err == nil {
		t.Error("expected a non-zip file to be refused")
	}
}

func TestResolveDescriptorPrefersTheSiblingStubOverAContentRootDescriptor(t *testing.T) {
	dir := t.TempDir()
	// Even with a descriptor.mod also present in the content root, a real sibling
	// stub (the stronger signal - see ExtractResult.StubDescriptor) wins.
	if err := os.WriteFile(filepath.Join(dir, "descriptor.mod"), []byte(`name="From content root"`), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := []byte("name=\"From sibling stub\"\nversion=\"3.0\"\n")

	got, hadOwn, err := ResolveDescriptor(dir, stub, "Fallback Title", "31347")
	if err != nil {
		t.Fatalf("ResolveDescriptor: %v", err)
	}
	if !hadOwn {
		t.Error("expected hadOwnDescriptor to be true")
	}
	if got.Name != "From sibling stub" {
		t.Errorf("Name = %q, want the sibling stub's own name to win", got.Name)
	}
	if got.Version != "3.0" {
		t.Errorf("Version = %q, want the sibling stub's own version", got.Version)
	}
	if got.RemoteFileID != "31347" || got.Path != dir {
		t.Errorf("got %+v", got)
	}
}

func TestResolveDescriptorUsesTheArchivesOwnDescriptor(t *testing.T) {
	dir := t.TempDir()
	desc := "name=\"Author's Mod\"\nversion=\"2.0\"\ntags={\n\t\"Gameplay\"\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "descriptor.mod"), []byte(desc), 0o644); err != nil {
		t.Fatal(err)
	}

	got, hadOwn, err := ResolveDescriptor(dir, nil, "Fallback Title", "31347")
	if err != nil {
		t.Fatalf("ResolveDescriptor: %v", err)
	}
	if !hadOwn {
		t.Error("expected hadOwnDescriptor to be true")
	}
	if got.Name != "Author's Mod" {
		t.Errorf("Name = %q, want the author's own name kept", got.Name)
	}
	if got.Version != "2.0" {
		t.Errorf("Version = %q, want the author's own version kept", got.Version)
	}
	if got.RemoteFileID != "31347" {
		t.Errorf("RemoteFileID = %q, want it set to the LoversLab file id regardless", got.RemoteFileID)
	}
	if got.Path != dir {
		t.Errorf("Path = %q, want %q", got.Path, dir)
	}
}

func TestResolveDescriptorSynthesizesOneWhenTheArchiveHasNone(t *testing.T) {
	dir := t.TempDir()
	got, hadOwn, err := ResolveDescriptor(dir, nil, "Fallback Title", "31347")
	if err != nil {
		t.Fatalf("ResolveDescriptor: %v", err)
	}
	if hadOwn {
		t.Error("expected hadOwnDescriptor to be false")
	}
	if got.Name != "Fallback Title" || got.Path != dir || got.RemoteFileID != "31347" {
		t.Errorf("got %+v", got)
	}
}

func TestResolveDescriptorFallsBackToTitleWhenTheArchivesOwnNameIsBlank(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "descriptor.mod"), []byte(`version="1.0"`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, hadOwn, err := ResolveDescriptor(dir, nil, "Fallback Title", "31347")
	if err != nil {
		t.Fatalf("ResolveDescriptor: %v", err)
	}
	if !hadOwn {
		t.Error("expected hadOwnDescriptor to be true (the file existed, even with a blank name)")
	}
	if got.Name != "Fallback Title" {
		t.Errorf("Name = %q, want the fallback title used when the archive's own descriptor left it blank", got.Name)
	}
}

func TestResolveDescriptorErrorsOnACorruptExistingDescriptor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "descriptor.mod"), []byte("not valid { clausewitz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveDescriptor(dir, nil, "Fallback Title", "31347"); err == nil {
		t.Error("expected a corrupt existing descriptor.mod to be an error, not silently replaced")
	}
}

func TestResolveDescriptorErrorsOnACorruptSiblingStub(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := ResolveDescriptor(dir, []byte("not valid { clausewitz"), "Fallback Title", "31347"); err == nil {
		t.Error("expected a corrupt sibling stub to be an error, not silently replaced")
	}
}
