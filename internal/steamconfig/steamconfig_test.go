package steamconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Official-Husko/parallax-mod-manager/internal/steamapi"
)

func TestMissingFileIsDefaults(t *testing.T) {
	st, err := Store{Path: filepath.Join(t.TempDir(), FileName)}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st != Defaults() || st.Mode != steamapi.ModeFree {
		t.Errorf("st = %+v, want free with no key", st)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), FileName)}
	want := Settings{Mode: steamapi.ModeBackup, SealedKey: "v1.abc.def", Fingerprint: "0a1b2c3d"}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got != want {
		t.Errorf("Load = %+v, %v, want %+v", got, err, want)
	}
}

func TestFreeModeDeletesTheKeyFromTheFile(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), FileName)}
	if err := s.Save(Settings{Mode: steamapi.ModeComplete, SealedKey: "v1.abc.def", Fingerprint: "0a1b2c3d"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Settings{Mode: steamapi.ModeFree, SealedKey: "v1.abc.def", Fingerprint: "0a1b2c3d"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.Path)
	if strings.Contains(string(data), "v1.abc.def") || strings.Contains(string(data), "0a1b2c3d") {
		t.Errorf("free mode left the key in the file:\n%s", data)
	}
	got, _ := s.Load()
	if got != Defaults() {
		t.Errorf("Load = %+v, want defaults", got)
	}
}

func TestKeyModeWithoutAKeyBecomesFree(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), FileName)}
	if err := s.Save(Settings{Mode: steamapi.ModeComplete}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(); got.Mode != steamapi.ModeFree {
		t.Errorf("Mode = %q, want free (no key to use)", got.Mode)
	}
}

func TestFileHasCommentsAndNoPlainKey(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), FileName)}
	_ = s.Save(Settings{Mode: steamapi.ModeComplete, SealedKey: "v1.abc.def", Fingerprint: "0a1b2c3d"})
	data, _ := os.ReadFile(s.Path)
	text := string(data)
	if !strings.Contains(text, "// How the key is used") || !strings.Contains(text, "ENCRYPTED") {
		t.Errorf("the file has no explanation beside its fields:\n%s", text)
	}
	if strings.Contains(text, "—") {
		t.Error("the file contains an em dash")
	}
}

func TestSavedFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	s := Store{Path: filepath.Join(t.TempDir(), FileName)}
	_ = s.Save(Settings{Mode: steamapi.ModeComplete, SealedKey: "v1.abc.def", Fingerprint: "0a1b2c3d"})
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestCorruptFileFallsBackToDefaultsWithAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	_ = os.WriteFile(path, []byte("{ this is not json"), 0o600)
	st, err := Store{Path: path}.Load()
	if err == nil {
		t.Error("want an error for a corrupt file")
	}
	if st != Defaults() {
		t.Errorf("st = %+v, want defaults", st)
	}
}

func TestHandEditedJSONCIsAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	_ = os.WriteFile(path, []byte("// mine\n{\n \"mode\": \"backup\", /* why */\n \"key\": \"v1.a.b\",\n \"fingerprint\": \"deadbeef\",\n}\n"), 0o600)
	st, err := Store{Path: path}.Load()
	if err != nil || st.Mode != steamapi.ModeBackup || st.SealedKey != "v1.a.b" {
		t.Errorf("st = %+v, err = %v", st, err)
	}
}

func TestNoPathMeansNoSaving(t *testing.T) {
	s := Store{}
	if st, err := s.Load(); err != nil || st != Defaults() {
		t.Errorf("Load = %+v, %v", st, err)
	}
	if err := s.Save(Defaults()); err == nil {
		t.Error("Save with no path succeeded")
	}
}
