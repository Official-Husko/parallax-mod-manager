package modedit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/mod"
)

func TestFolderName(t *testing.T) {
	cases := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{name: "My Mod", want: "My Mod"},
		{name: `weird/name\with:many*bad?chars"in<it>here|too`, want: "weird_name_with_many_bad_chars_in_it_here_too"},
		{name: "  padded  ", want: "padded"},
		{name: "", want: "_"},
		{name: "   ", want: "_"},
		{name: ".", wantErr: true},
		{name: "..", wantErr: true},
	}
	for _, c := range cases {
		got, err := FolderName(c.name)
		if c.wantErr {
			if err == nil {
				t.Errorf("FolderName(%q) = %q, want an error", c.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("FolderName(%q): unexpected error: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("FolderName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestNewFilesWritesADescriptorWithNoPathAndAStubWithOne(t *testing.T) {
	want := Fields{Name: "My New Mod", Version: "1.0", SupportedVersion: "v4.*", Tags: []string{"Gameplay"}}
	contentDir := filepath.Join(t.TempDir(), "content", "My New Mod")
	stubPath := filepath.Join(t.TempDir(), "mod", "My New Mod.mod")

	edits, err := NewFiles(want, contentDir, stubPath)
	if err != nil {
		t.Fatalf("NewFiles: %v", err)
	}
	if len(edits) != 2 {
		t.Fatalf("got %d edits, want 2", len(edits))
	}

	desc := edits[0]
	if desc.Kind != KindDescriptor || !desc.Create || desc.Path != filepath.Join(contentDir, "descriptor.mod") {
		t.Errorf("descriptor edit = %+v, unexpected", desc)
	}
	if strings.Contains(desc.After, "path=") {
		t.Errorf("descriptor.mod should not have a path= line, got:\n%s", desc.After)
	}

	stub := edits[1]
	if stub.Kind != KindStub || !stub.Create || stub.Path != stubPath {
		t.Errorf("stub edit = %+v, unexpected", stub)
	}
	parsed, err := mod.ParseDescriptor([]byte(stub.After), mod.DescriptorClassic)
	if err != nil {
		t.Fatalf("parsing stub: %v", err)
	}
	if parsed.Path != contentDir {
		t.Errorf("stub path = %q, want %q", parsed.Path, contentDir)
	}
	if parsed.Name != want.Name || parsed.Version != want.Version {
		t.Errorf("stub fields = %+v, want name/version %q/%q", parsed, want.Name, want.Version)
	}
}

func TestNewFilesWithoutAStubPathWritesOnlyTheDescriptor(t *testing.T) {
	want := Fields{Name: "Extra Folder Mod"}
	contentDir := filepath.Join(t.TempDir(), "Extra Folder Mod")

	edits, err := NewFiles(want, contentDir, "")
	if err != nil {
		t.Fatalf("NewFiles: %v", err)
	}
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1 (no stub)", len(edits))
	}
	if edits[0].Kind != KindDescriptor {
		t.Errorf("edit kind = %q, want descriptor", edits[0].Kind)
	}
}

func TestNewFilesRefusesAnUnrepresentableField(t *testing.T) {
	want := Fields{Name: "Bad\nName"}
	if _, err := NewFiles(want, t.TempDir(), ""); err == nil {
		t.Error("NewFiles with a newline in the name succeeded, want an error")
	}
}

func TestNewFilesRefusesABlankName(t *testing.T) {
	if _, err := NewFiles(Fields{}, t.TempDir(), ""); err == nil {
		t.Error("NewFiles with no name succeeded, want an error")
	}
}
