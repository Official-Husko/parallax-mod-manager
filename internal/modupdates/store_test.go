package modupdates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFingerprintDirChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("descriptor.mod", "name=x")
	write("common/a.txt", "12345")

	first := FingerprintDir(dir)
	if !first.Known || first.Files != 2 || first.Size != int64(len("name=x")+5) || first.Newest == 0 {
		t.Fatalf("first = %+v", first)
	}
	if again := FingerprintDir(dir); again != first {
		t.Errorf("same content, different fingerprint: %+v vs %+v", again, first)
	}

	write("common/b.txt", "more")
	second := FingerprintDir(dir)
	if second.Files != 3 || second.Size != first.Size+4 {
		t.Errorf("after adding a file = %+v, want 3 files and 4 more bytes", second)
	}

	if err := os.Chtimes(filepath.Join(dir, "common/a.txt"), second.newestTime().Add(3600e9), second.newestTime().Add(3600e9)); err != nil {
		t.Fatal(err)
	}
	if third := FingerprintDir(dir); third == second {
		t.Error("touching a file did not change the fingerprint")
	}
}

func TestFingerprintDirUnreadableRootIsUnknown(t *testing.T) {
	if f := FingerprintDir(filepath.Join(t.TempDir(), "missing")); f.Known {
		t.Errorf("missing folder = %+v, want unknown", f)
	}
	if f := FingerprintDir(""); f.Known {
		t.Errorf("empty path = %+v, want unknown", f)
	}
}

func TestStoreRoundTripAndHeader(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	want := snap(123, map[string]Record{
		"ugc_1": {Name: "One", Source: SourceWorkshop, Version: "1.0", RemoteFileID: "1", Content: fp(2, 20, 3), WorkshopUpdated: 9, WorkshopGone: true, GoneSince: 8},
	})
	if err := s.Save("stellaris", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Dir, "stellaris.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "// Parallax Mod Manager") {
		t.Errorf("file does not start with its explanatory comment:\n%s", raw)
	}
	got := s.Load("stellaris")
	if got == nil || got.TakenAt != 123 || got.Mods["ugc_1"] != want.Mods["ugc_1"] {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestStoreLoadDegradesToNil(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if s.Load("nope") != nil {
		t.Error("missing file should load as nil")
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.jsonc"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s.Load("bad") != nil {
		t.Error("corrupt file should load as nil")
	}
	if (Store{}).Load("x") != nil {
		t.Error("empty Dir should load as nil")
	}
	if err := (Store{}).Save("x", Snapshot{}); err == nil {
		t.Error("Save with an empty Dir succeeded, want an error")
	}
}

func TestStoreIsPerGame(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save("stellaris", snap(1, map[string]Record{"a": {Name: "A"}})); err != nil {
		t.Fatal(err)
	}
	if s.Load("hoi4") != nil {
		t.Error("hoi4 loaded stellaris's snapshot")
	}
}
