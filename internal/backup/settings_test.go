package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseModeDefaultsToAtRisk(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeAtRisk, "junk": ModeAtRisk, "atrisk": ModeAtRisk, "all": ModeAll, "off": ModeOff} {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultRootIsParallaxModBackupsInHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder")
	}
	if got, want := DefaultRoot(), filepath.Join(home, "Parallax Mod Backups"); got != want {
		t.Errorf("DefaultRoot = %q, want %q", got, want)
	}
	if got := (Settings{}).Root(); got != DefaultRoot() {
		t.Errorf("empty path Root = %q, want the default", got)
	}
	if got := (Settings{Path: "  /somewhere/else/  "}).Root(); got != filepath.Clean("/somewhere/else") {
		t.Errorf("custom path Root = %q", got)
	}
}

func TestSettingsRoundTripAndComments(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), SettingsFile)}
	if st, err := s.Load(); err != nil || st != Defaults() {
		t.Fatalf("missing file: %+v, %v", st, err)
	}
	want := Settings{Mode: ModeAll, Path: "/mnt/big drive/Mod Backups"}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got != want {
		t.Errorf("Load = %+v, %v, want %+v", got, err, want)
	}
	data, _ := os.ReadFile(s.Path)
	text := string(data)
	if !strings.Contains(text, "// Which mods are backed up on their own") || !strings.Contains(text, "// Where backups are kept") || strings.Contains(text, "—") {
		t.Errorf("the file has no explanation beside its fields:\n%s", text)
	}
}

func TestCorruptSettingsFallBackToDefaultsWithAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), SettingsFile)
	_ = os.WriteFile(path, []byte("{ nope"), 0o644)
	st, err := Store{Path: path}.Load()
	if err == nil || st != Defaults() {
		t.Errorf("Load = %+v, %v", st, err)
	}
	if err := (Store{}).Save(Defaults()); err == nil {
		t.Error("Save with no path succeeded")
	}
}

func TestValidateRoot(t *testing.T) {
	if err := ValidateRoot("relative/path"); err == nil {
		t.Error("a relative path was accepted")
	}
	dir := filepath.Join(t.TempDir(), "new", "nested")
	if err := ValidateRoot(dir); err != nil {
		t.Fatalf("a creatable folder was refused: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Error("the folder was not created")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the write test left files behind: %v", entries)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		ro := filepath.Join(t.TempDir(), "ro")
		_ = os.MkdirAll(ro, 0o755)
		_ = os.Chmod(ro, 0o555)
		defer os.Chmod(ro, 0o755)
		if err := ValidateRoot(ro); err == nil {
			t.Error("a read-only folder was accepted")
		}
	}
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if err := ValidateRoot(file); err == nil {
		t.Error("a file was accepted as the backup folder")
	}
}
